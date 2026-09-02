import { useEffect, useState } from 'react';
import { client } from '../../api/client';
import { useToast } from '../../components/ui/Toast';
import Card from '../../components/ui/Card';
import Button from '../../components/ui/Button';
import Input from '../../components/ui/Input';
import Modal from '../../components/ui/Modal';
import Skeleton from '../../components/ui/Skeleton';
import Pagination from '../../components/ui/Pagination';
import { Badge } from '../../components/ui/Badge';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '../../components/ui/Table';
import { Plus, Trash2, CreditCard, Receipt, Search, RefreshCw, Check, CheckSquare, Square } from 'lucide-react';

function generateLuhnCard(bin, length = 16) {
  let card = bin.trim().replace(/\D/g, '');
  if (!card) card = '400000'; // Fallback prefix if empty
  
  // Append random digits up to length - 1
  while (card.length < length - 1) {
    card += Math.floor(Math.random() * 10);
  }
  
  // Calculate Luhn check digit
  let sum = 0;
  let shouldDouble = true;
  for (let i = card.length - 1; i >= 0; i--) {
    let digit = parseInt(card.charAt(i), 10);
    if (shouldDouble) {
      digit *= 2;
      if (digit > 9) digit -= 9;
    }
    sum += digit;
    shouldDouble = !shouldDouble;
  }
  
  const checkDigit = (10 - (sum % 10)) % 10;
  return card + checkDigit;
}

export default function VCC() {
  const [cards, setCards] = useState([]);
  const [transactions, setTransactions] = useState([]);
  const [loading, setLoading] = useState(true);
  const [cardSearch, setCardSearch] = useState('');
  const [txSearch, setTxSearch] = useState('');
  const [cardPage, setCardPage] = useState(0);
  const [txPage, setTxPage] = useState(0);
  const [modalOpen, setModalOpen] = useState(false);
  const [form, setForm] = useState({ number: '', bin: '', exp_month: '', exp_year: '', cvv: '', name: '' });
  
  // BIN Generator States
  const [genModalOpen, setGenModalOpen] = useState(false);
  const [genBin, setGenBin] = useState('411111');
  const [genMonth, setGenMonth] = useState('');
  const [genYear, setGenYear] = useState('');
  const [genCVV, setGenCVV] = useState('');
  const [genName, setGenName] = useState('Switchblade User');
  const [genQty, setGenQty] = useState(10);
  const [generatedList, setGeneratedList] = useState([]);
  const [selectedGenIndices, setSelectedGenIndices] = useState([]);
  const [savingGen, setSavingGen] = useState(false);

  const toast = useToast();
  const PAGE_SIZE = 5;

  useEffect(() => { load(); }, []);

  async function load() {
    try {
      const [cardRes, txRes] = await Promise.all([
        client.get('/api/vcc/cards').catch(() => ({ data: [] })),
        client.get('/api/vcc/transactions').catch(() => ({ data: [] })),
      ]);
      setCards(Array.isArray(cardRes.data) ? cardRes.data : []);
      setTransactions(Array.isArray(txRes.data) ? txRes.data : []);
    } catch {
      toast.error('Failed to load VCC data');
    } finally {
      setLoading(false);
    }
  }

  async function addCard() {
    try {
      await client.post('/api/vcc/cards', form);
      toast('Card added successfully');
      setModalOpen(false);
      setForm({ number: '', bin: '', exp_month: '', exp_year: '', cvv: '', name: '' });
      load();
    } catch {
      toast.error('Failed to add card');
    }
  }

  async function removeCard(id) {
    if (!window.confirm('Delete this virtual card?')) return;
    try {
      await client.delete(`/api/vcc/cards/${id}`);
      toast('Card deleted');
      load();
    } catch {
      toast.error('Delete failed');
    }
  }

  // Generate cards locally based on BIN
  function handleGenerate() {
    const list = [];
    const sanitizedBin = genBin.trim().replace(/\D/g, '');
    const cleanBin = sanitizedBin.substring(0, 6) || '411111';
    
    const qty = Math.max(1, Math.min(50, Number(genQty) || 10));
    
    for (let i = 0; i < qty; i++) {
      const number = generateLuhnCard(cleanBin);
      const expMonth = genMonth ? String(Number(genMonth)).padStart(2, '0') : String(Math.floor(Math.random() * 12) + 1).padStart(2, '0');
      const expYear = genYear ? String(Number(genYear)) : String(new Date().getFullYear() + Math.floor(Math.random() * 5) + 2);
      const cvv = genCVV ? String(Number(genCVV)).padStart(3, '0') : String(Math.floor(Math.random() * 900) + 100);
      
      list.push({
        number,
        bin: cleanBin,
        exp_month: expMonth,
        exp_year: expYear,
        cvv,
        name: genName || 'Switchblade User',
      });
    }
    
    setGeneratedList(list);
    // Auto-select all by default
    setSelectedGenIndices(Array.from({ length: list.length }, (_, idx) => idx));
    toast(`Generated ${list.length} cards conforming to Luhn algorithm`);
  }

  // Import selected generated cards
  async function handleImportGenerated() {
    const toImport = generatedList.filter((_, idx) => selectedGenIndices.includes(idx));
    if (toImport.length === 0) {
      toast.error('No cards selected to import');
      return;
    }
    
    setSavingGen(true);
    try {
      await Promise.all(toImport.map((card) => client.post('/api/vcc/cards', card)));
      toast(`Successfully imported ${toImport.length} cards to VCC pool`);
      setGenModalOpen(false);
      setGeneratedList([]);
      setSelectedGenIndices([]);
      load();
    } catch {
      toast.error('Failed to import generated cards');
    } finally {
      setSavingGen(false);
    }
  }

  function toggleSelectGen(idx) {
    if (selectedGenIndices.includes(idx)) {
      setSelectedGenIndices(selectedGenIndices.filter((i) => i !== idx));
    } else {
      setSelectedGenIndices([...selectedGenIndices, idx]);
    }
  }

  function toggleSelectAllGen() {
    if (selectedGenIndices.length === generatedList.length) {
      setSelectedGenIndices([]);
    } else {
      setSelectedGenIndices(Array.from({ length: generatedList.length }, (_, i) => i));
    }
  }

  function maskNumber(num) {
    if (!num) return '—';
    return `•••• ${String(num).slice(-4)}`;
  }

  const filteredCards = cards.filter((c) => {
    const q = cardSearch.trim().toLowerCase();
    if (!q) return true;
    return (
      (c.number_masked || '').toLowerCase().includes(q) ||
      (c.name || '').toLowerCase().includes(q) ||
      (c.status || '').toLowerCase().includes(q)
    );
  });

  const totalCardPages = Math.ceil(filteredCards.length / PAGE_SIZE);
  const cardPageData = filteredCards.slice(cardPage * PAGE_SIZE, (cardPage + 1) * PAGE_SIZE);

  const filteredTransactions = transactions.filter((t) => {
    const q = txSearch.trim().toLowerCase();
    if (!q) return true;
    return (
      (t.card_last4 || '').toLowerCase().includes(q) ||
      (t.card_brand || '').toLowerCase().includes(q) ||
      (t.status || '').toLowerCase().includes(q) ||
      (t.currency || '').toLowerCase().includes(q)
    );
  });

  const totalTxPages = Math.ceil(filteredTransactions.length / PAGE_SIZE);
  const txPageData = filteredTransactions.slice(txPage * PAGE_SIZE, (txPage + 1) * PAGE_SIZE);

  return (
    <div className="space-y-6">
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold text-text">VCC Pool</h1>
          <p className="text-sm text-muted mt-1">Virtual credit cards used for upstream account subscriptions</p>
        </div>
        <div className="flex items-center gap-2">
          <Button size="sm" variant="secondary" onClick={() => { handleGenerate(); setGenModalOpen(true); }}>
            <RefreshCw className="w-3.5 h-3.5 mr-1" /> BIN Generator
          </Button>
          <Button size="sm" onClick={() => setModalOpen(true)}>
            <Plus className="w-3.5 h-3.5 mr-1" /> Add Card
          </Button>
        </div>
      </div>

      <Card>
        <div className="flex flex-col gap-4 md:flex-row md:items-center md:justify-between mb-4">
          <div className="flex items-center gap-2">
            <CreditCard className="w-4 h-4 text-primary" />
            <h3 className="text-base font-semibold text-text">Cards Pool</h3>
          </div>
          {cards.length > 0 && (
            <div className="max-w-xs w-full">
              <Input
                placeholder="Search cards..."
                value={cardSearch}
                onChange={(e) => { setCardSearch(e.target.value); setCardPage(0); }}
                icon={<Search className="w-4 h-4 text-muted/50" />}
              />
            </div>
          )}
        </div>
        {loading ? (
          <div className="space-y-2">
            {Array.from({ length: 3 }).map((_, i) => (
              <Skeleton key={i} className="h-12 w-full" />
            ))}
          </div>
        ) : filteredCards.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-12 gap-2 text-muted">
            <CreditCard className="w-8 h-8 text-muted/30" />
            <p className="text-sm">{cardSearch ? 'No cards match search criteria.' : 'No VCC cards added'}</p>
          </div>
        ) : (
          <>
            <Table>
              <TableHead>
                <TableRow>
                  <TableHeader>Number</TableHeader>
                  <TableHeader>BIN Prefix</TableHeader>
                  <TableHeader>Expires</TableHeader>
                  <TableHeader>Success / Fail</TableHeader>
                  <TableHeader>Status</TableHeader>
                  <TableHeader className="w-24 text-right">Actions</TableHeader>
                </TableRow>
              </TableHead>
              <TableBody>
                {cardPageData.map((c) => (
                  <TableRow key={c.id}>
                    <TableCell className="font-mono text-xs text-text-secondary select-all font-semibold">{c.number_masked || '—'}</TableCell>
                    <TableCell className="font-mono text-xs text-muted select-all">{c.bin || '—'}</TableCell>
                    <TableCell className="text-xs text-muted font-semibold">{c.exp_month}/{c.exp_year}</TableCell>
                    <TableCell className="tabular-nums font-semibold text-xs text-muted">{c.success_count ?? 0} / {c.fail_count ?? 0}</TableCell>
                    <TableCell>
                      <Badge variant={c.status === 'active' ? 'success' : 'default'}>{c.status || 'unknown'}</Badge>
                    </TableCell>
                    <TableCell>
                      <div className="flex justify-end">
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => removeCard(c.id)}
                          className="text-danger hover:text-danger hover:bg-danger/10 p-1.5 h-8 w-8"
                          title="Delete card from pool"
                        >
                          <Trash2 className="w-4 h-4" />
                        </Button>
                      </div>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>

            <Pagination
              currentPage={cardPage}
              totalPages={totalCardPages}
              onPageChange={setCardPage}
            />
          </>
        )}
      </Card>

      <Card>
        <div className="flex flex-col gap-4 md:flex-row md:items-center md:justify-between mb-4">
          <div className="flex items-center gap-2">
            <Receipt className="w-4 h-4 text-primary" />
            <h3 className="text-base font-semibold text-text">Recent Transactions</h3>
          </div>
          {transactions.length > 0 && (
            <div className="max-w-xs w-full">
              <Input
                placeholder="Search transactions..."
                value={txSearch}
                onChange={(e) => { setTxSearch(e.target.value); setTxPage(0); }}
                icon={<Search className="w-4 h-4 text-muted/50" />}
              />
            </div>
          )}
        </div>
        {loading ? (
          <div className="space-y-2">
            {Array.from({ length: 3 }).map((_, i) => (
              <Skeleton key={i} className="h-12 w-full" />
            ))}
          </div>
        ) : filteredTransactions.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-12 gap-2 text-muted">
            <Receipt className="w-8 h-8 text-muted/30" />
            <p className="text-sm">{txSearch ? 'No transactions match search criteria.' : 'No transactions yet'}</p>
          </div>
        ) : (
          <>
            <Table>
              <TableHead>
                <TableRow>
                  <TableHeader>Card Number</TableHeader>
                  <TableHeader>Brand</TableHeader>
                  <TableHeader>Amount</TableHeader>
                  <TableHeader>Status</TableHeader>
                  <TableHeader>Date</TableHeader>
                </TableRow>
              </TableHead>
              <TableBody>
                {txPageData.map((t) => (
                  <TableRow key={t.id}>
                    <TableCell className="font-mono text-xs text-text-secondary">{t.card_last4 ? maskNumber(t.card_last4) : '—'}</TableCell>
                    <TableCell className="text-xs text-muted font-medium capitalize">{t.card_brand || '—'}</TableCell>
                    <TableCell className="tabular-nums font-semibold text-xs text-text-secondary">
                      {t.amount == null ? '—' : `${t.currency || ''} ${t.amount.toFixed(2)}`}
                    </TableCell>
                    <TableCell>
                      <Badge variant={t.status === 'success' ? 'success' : 'default'}>{t.status || 'unknown'}</Badge>
                    </TableCell>
                    <TableCell className="text-xs text-muted">
                      {t.created_at ? new Date(t.created_at * 1000).toLocaleString() : '—'}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>

            <Pagination
              currentPage={txPage}
              totalPages={totalTxPages}
              onPageChange={setTxPage}
            />
          </>
        )}
      </Card>

      {/* MODAL 1: Add Card Manually */}
      <Modal open={modalOpen} onClose={() => setModalOpen(false)} title="Add Card Manually">
        <div className="space-y-4">
          <Input label="Card Number" value={form.number} onChange={(e) => setForm({ ...form, number: e.target.value })} placeholder="4111222233334444" required />
          <div className="grid grid-cols-2 gap-4">
            <Input label="BIN" value={form.bin} onChange={(e) => setForm({ ...form, bin: e.target.value })} placeholder="411122" />
            <Input label="CVV" value={form.cvv} onChange={(e) => setForm({ ...form, cvv: e.target.value })} placeholder="123" required />
          </div>
          <div className="grid grid-cols-2 gap-4">
            <Input label="Exp Month" type="number" min="1" max="12" value={form.exp_month} onChange={(e) => setForm({ ...form, exp_month: e.target.value })} placeholder="12" required />
            <Input label="Exp Year" type="number" min="2026" max="2040" value={form.exp_year} onChange={(e) => setForm({ ...form, exp_year: e.target.value })} placeholder="2028" required />
          </div>
          <Input label="Cardholder Name" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} placeholder="John Doe" />
          <div className="flex gap-2 justify-end pt-2">
            <Button variant="ghost" onClick={() => setModalOpen(false)}>Cancel</Button>
            <Button onClick={addCard} disabled={!form.number || !form.cvv || !form.exp_month || !form.exp_year} variant="primary">Add Card</Button>
          </div>
        </div>
      </Modal>

      {/* MODAL 2: BIN Generator */}
      <Modal open={genModalOpen} onClose={() => setGenModalOpen(false)} title="BIN Card Generator" width="max-w-3xl">
        <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
          {/* Controls Form */}
          <div className="md:col-span-1 space-y-4 pr-0 md:pr-4 border-r-0 md:border-r border-border/60">
            <Input
              label="BIN Pattern (6 Digits)"
              value={genBin}
              onChange={(e) => setGenBin(e.target.value.replace(/\D/g, ''))}
              placeholder="411111"
              maxLength={6}
              required
            />
            <div className="grid grid-cols-2 gap-2">
              <Input
                label="Exp Month"
                value={genMonth}
                onChange={(e) => setGenMonth(e.target.value.replace(/\D/g, ''))}
                placeholder="Rand"
                maxLength={2}
              />
              <Input
                label="Exp Year"
                value={genYear}
                onChange={(e) => setGenYear(e.target.value.replace(/\D/g, ''))}
                placeholder="Rand"
                maxLength={4}
              />
            </div>
            <div className="grid grid-cols-2 gap-2">
              <Input
                label="CVV"
                value={genCVV}
                onChange={(e) => setGenCVV(e.target.value.replace(/\D/g, ''))}
                placeholder="Rand"
                maxLength={4}
              />
              <Input
                label="Qty"
                type="number"
                value={genQty}
                onChange={(e) => setGenQty(Math.max(1, Math.min(50, Number(e.target.value))))}
                placeholder="10"
              />
            </div>
            <Input
              label="Cardholder Name"
              value={genName}
              onChange={(e) => setGenName(e.target.value)}
              placeholder="Switchblade User"
            />
            <Button onClick={handleGenerate} className="w-full font-semibold gap-1">
              <RefreshCw className="w-3.5 h-3.5" />
              Generate
            </Button>
          </div>

          {/* Preview Panel */}
          <div className="md:col-span-2 space-y-4">
            <div className="flex items-center justify-between border-b border-border/40 pb-2">
              <span className="text-sm font-bold text-text-secondary">
                Generated Cards Preview ({generatedList.length})
              </span>
              {generatedList.length > 0 && (
                <button
                  onClick={toggleSelectAllGen}
                  className="text-xs text-primary font-bold hover:underline select-none"
                >
                  {selectedGenIndices.length === generatedList.length ? 'Deselect All' : 'Select All'}
                </button>
              )}
            </div>

            {generatedList.length === 0 ? (
              <div className="py-20 text-center text-muted text-xs">
                No cards generated. Adjust BIN parameters and click "Generate".
              </div>
            ) : (
              <>
                <div className="max-h-72 overflow-y-auto border border-border/40 rounded-lg bg-surface-elevated/40">
                  <table className="w-full text-left text-xs border-collapse">
                    <thead>
                      <tr className="border-b border-border/40 bg-surface-hover/30 text-muted">
                        <th className="p-2 w-8 text-center"></th>
                        <th className="p-2">Card Number</th>
                        <th className="p-2 w-16">Expiry</th>
                        <th className="p-2 w-10">CVV</th>
                        <th className="p-2 truncate">Name</th>
                      </tr>
                    </thead>
                    <tbody>
                      {generatedList.map((card, idx) => {
                        const isSelected = selectedGenIndices.includes(idx);
                        return (
                          <tr
                            key={idx}
                            onClick={() => toggleSelectGen(idx)}
                            className="border-b border-border/20 last:border-0 hover:bg-surface-hover/40 cursor-pointer"
                          >
                            <td className="p-2 text-center" onClick={(e) => e.stopPropagation()}>
                              <button onClick={() => toggleSelectGen(idx)} className="text-muted hover:text-primary">
                                {isSelected ? (
                                  <CheckSquare className="w-4 h-4 text-primary" />
                                ) : (
                                  <Square className="w-4 h-4 text-muted/30" />
                                )}
                              </button>
                            </td>
                            <td className="p-2 font-mono font-semibold select-all text-text-secondary">{card.number}</td>
                            <td className="p-2 font-mono text-muted">{card.exp_month}/{card.exp_year}</td>
                            <td className="p-2 font-mono text-muted">{card.cvv}</td>
                            <td className="p-2 text-muted truncate max-w-[120px]" title={card.name}>{card.name}</td>
                          </tr>
                        );
                      })}
                    </tbody>
                  </table>
                </div>

                <div className="flex gap-2 justify-end pt-2">
                  <Button variant="ghost" onClick={() => setGenModalOpen(false)}>
                    Close
                  </Button>
                  <Button
                    onClick={handleImportGenerated}
                    disabled={selectedGenIndices.length === 0}
                    loading={savingGen}
                    variant="primary"
                    className="font-semibold"
                  >
                    Import Selected ({selectedGenIndices.length})
                  </Button>
                </div>
              </>
            )}
          </div>
        </div>
      </Modal>
    </div>
  );
}
