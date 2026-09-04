package api

import (
	"math/rand"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

// MountToolsAPI mounts utility tool endpoints.
func MountToolsAPI(r chi.Router) {
	r.Get("/api/tools/random-identity", handleRandomIdentity)
	r.Get("/api/tools/password", handleRandomPassword)
}

var (
	firstNames = []string{"James", "Emma", "Liam", "Olivia", "Noah", "Ava", "Lucas", "Sophia", "Mason", "Isabella",
		"Ethan", "Mia", "Logan", "Charlotte", "Aiden", "Amelia", "Jackson", "Harper", "Caleb", "Evelyn",
		"Akira", "Yuki", "Min-Jun", "Soo-Yun", "Wei", "Mei", "Raj", "Priya", "Arjun", "Ananya",
		"Omar", "Aisha", "Khalid", "Fatima", "Diego", "Sofia", "Mateo", "Valentina", "Lucas", "Camila"}

	lastNames = []string{"Smith", "Johnson", "Williams", "Brown", "Jones", "Garcia", "Miller", "Davis", "Rodriguez",
		"Martinez", "Hernandez", "Lopez", "Gonzalez", "Wilson", "Anderson", "Thomas", "Taylor", "Moore",
		"Jackson", "Martin", "Lee", "Perez", "Thompson", "White", "Harris", "Sanchez", "Clark", "Ramirez",
		"Tanaka", "Kim", "Patel", "Singh", "Kumar", "Okafor", "Adeyemi", "Ibrahim", "Suzuki", "Chen",
		"Wang", "Liu", "Zhang", "Li", "Yang", "Huang", "Zhao", "Wu", "Zhou"}

	domains = []string{"gmail.com", "outlook.com", "yahoo.com", "protonmail.com", "icloud.com",
		"hotmail.com", "fastmail.com", "zoho.com", "mail.com", "gmx.com"}

	streets = []string{"Main St", "Oak Ave", "Pine Dr", "Maple Ln", "Cedar Blvd", "Elm St", "Washington Ave",
		"Park Dr", "Lake Rd", "Hill St", "Sunset Blvd", "River Rd", "Highland Ave", "Forest Dr"}

	cities = []string{"New York", "Los Angeles", "Chicago", "Houston", "Phoenix", "Philadelphia",
		"San Antonio", "San Diego", "Dallas", "San Jose", "Austin", "Seattle", "Denver", "Boston",
		"Portland", "Atlanta", "Miami", "Nashville", "Las Vegas", "Salt Lake City"}

	states = []string{"NY", "CA", "IL", "TX", "AZ", "PA", "FL", "WA", "CO", "MA", "OR", "GA", "NV", "UT"}

	countries = []string{"United States", "United Kingdom", "Canada", "Australia", "Germany", "France",
		"Japan", "Singapore", "Netherlands", "Sweden", "Brazil", "Mexico"}
)

func handleRandomIdentity(w http.ResponseWriter, r *http.Request) {
	fn := firstNames[rand.Intn(len(firstNames))]
	ln := lastNames[rand.Intn(len(lastNames))]
	email := fn + "." + ln + strconv.Itoa(rand.Intn(999)) + "@" + domains[rand.Intn(len(domains))]

	identity := map[string]string{
		"first_name": fn,
		"last_name":  ln,
		"full_name":  fn + " " + ln,
		"email":      email,
		"username":   fn + ln + strconv.Itoa(rand.Intn(9999)),
		"phone":      "+1" + strconv.FormatInt(2000000000+rand.Int63n(8000000000), 10),
		"street":     strconv.Itoa(rand.Intn(9999)) + " " + streets[rand.Intn(len(streets))],
		"city":       cities[rand.Intn(len(cities))],
		"state":      states[rand.Intn(len(states))],
		"zip":        strconv.Itoa(10000 + rand.Intn(89999)),
		"country":    countries[rand.Intn(len(countries))],
	}
	jsonOK(w, identity)
}

func handleRandomPassword(w http.ResponseWriter, r *http.Request) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*"
	length := 16
	if l := r.URL.Query().Get("length"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 128 {
			length = n
		}
	}
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	jsonOK(w, map[string]string{"password": string(b)})
}
