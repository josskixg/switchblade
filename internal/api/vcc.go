package api

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"switchblade/internal/config"
	"switchblade/internal/crypto"
	"switchblade/internal/db"
)

// MountVCCAPI registers VCC card and transaction endpoints.
func MountVCCAPI(r chi.Router, database *db.DB) {
	r.Get("/api/vcc/cards", listVCCCards(database))
	r.Post("/api/vcc/cards", RequireJSONHandler(createVCCCard(database)))
	r.Delete("/api/vcc/cards/{id}", deleteVCCCard(database))
	r.Get("/api/vcc/transactions", listVCCTransactions(database))
	r.Post("/api/vcc/auto-assign", RequireJSONHandler(vccAutoAssign(database)))
}

func listVCCCards(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := database.Query(
			`SELECT id, number, bin, exp_month, exp_year, name, status, success_count, fail_count, created_at FROM vcc_cards ORDER BY id DESC`)
		if err != nil {
			log.Printf("[api] list vcc cards failed: %v", err)
			jsonError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		defer rows.Close()

		var cards []map[string]any
		for rows.Next() {
			var id, successCount, failCount, createdAt int64
			var number, expMonth, expYear, status string
			var bin, name *string
			if err := rows.Scan(&id, &number, &bin, &expMonth, &expYear, &name, &status, &successCount, &failCount, &createdAt); err != nil {
				continue
			}
			// ponytail: mask card number — last4 only, no extra field
			start := len(number) - 4
			if start < 0 {
				start = 0
			}
			masked := "****-****-****-" + number[start:]
			cards = append(cards, map[string]any{
				"id": id, "number_masked": masked, "bin": bin,
				"exp_month": expMonth, "exp_year": expYear, "name": name,
				"status": status, "success_count": successCount,
				"fail_count": failCount, "created_at": createdAt,
			})
		}
		jsonOK(w, cards)
	}
}

func createVCCCard(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Number   string `json:"number"`
			Bin      string `json:"bin"`
			ExpMonth string `json:"exp_month"`
			ExpYear  string `json:"exp_year"`
			CVV      string `json:"cvv"`
			Name     string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			jsonError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if body.Number == "" || body.ExpMonth == "" || body.ExpYear == "" || body.CVV == "" {
			jsonError(w, http.StatusBadRequest, "number, exp_month, exp_year, cvv required")
			return
		}
		if body.Name == "" {
			body.Name = "John Doe"
		}
		now := time.Now().Unix()
		encryptedCVV := crypto.Encrypt(body.CVV, config.C.EncryptionKey)
		res, err := database.Exec(
			`INSERT INTO vcc_cards (number, bin, exp_month, exp_year, cvv, name, status, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, 'active', ?, ?)`,
			body.Number, body.Bin, body.ExpMonth, body.ExpYear, encryptedCVV, body.Name, now, now,
		)
		if err != nil {
			log.Printf("[api] create vcc card failed: %v", err)
			jsonError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		id, _ := res.LastInsertId()
		w.WriteHeader(http.StatusCreated)
		jsonOK(w, map[string]any{"id": id})
	}
}

func deleteVCCCard(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		_, err := database.Exec(`DELETE FROM vcc_cards WHERE id = ?`, id)
		if err != nil {
			log.Printf("[api] delete vcc card failed: %v", err)
			jsonError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		jsonOK(w, map[string]string{"status": "deleted"})
	}
}

func listVCCTransactions(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := database.Query(
			`SELECT id, account_id, card_id, card_last4, card_bin, card_brand, amount, currency, status, created_at
			 FROM vcc_transactions ORDER BY id DESC LIMIT 500`)
		if err != nil {
			log.Printf("[api] list vcc transactions failed: %v", err)
			jsonError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		defer rows.Close()

		var txns []map[string]any
		for rows.Next() {
			var id, createdAt int64
			var accountID, cardID *int64
			var cardLast4, cardBin, cardBrand, currency, status *string
			var amount *float64
			if err := rows.Scan(&id, &accountID, &cardID, &cardLast4, &cardBin, &cardBrand, &amount, &currency, &status, &createdAt); err != nil {
				continue
			}
			txns = append(txns, map[string]any{
				"id": id, "account_id": accountID, "card_id": cardID,
				"card_last4": cardLast4, "card_bin": cardBin, "card_brand": cardBrand,
				"amount": amount, "currency": currency, "status": status, "created_at": createdAt,
			})
		}
		jsonOK(w, txns)
	}
}

func vccAutoAssign(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Fetch up to 10 pending accounts.
		accRows, err := database.Query(
			`SELECT id FROM accounts WHERE status = 'pending' AND enabled = 1 LIMIT 10`)
		if err != nil {
			log.Printf("[api] vcc auto assign failed: %v", err)
			jsonError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		defer accRows.Close()

		var accountIDs []int64
		for accRows.Next() {
			var id int64
			if err := accRows.Scan(&id); err == nil {
				accountIDs = append(accountIDs, id)
			}
		}
		accRows.Close()

		assigned := 0
		noCards := 0
		now := time.Now().Unix()

		for _, accID := range accountIDs {
			// Pick an available card (active status, not yet assigned to anyone).
			row := database.QueryRow(
				`SELECT id FROM vcc_cards WHERE status = 'active' AND used_by_account_id IS NULL LIMIT 1`)
			var cardID int64
			if err := row.Scan(&cardID); err != nil {
				noCards++
				continue
			}
			_, err := database.Exec(
				`UPDATE vcc_cards SET used_by_account_id = ?, status = 'used', updated_at = ? WHERE id = ?`,
				accID, now, cardID)
			if err == nil {
				assigned++
			}
		}

		jsonOK(w, map[string]any{
			"assigned":           assigned,
			"no_cards_available": noCards,
		})
	}
}
