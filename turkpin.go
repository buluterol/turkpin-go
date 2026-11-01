// Package turkpin is a client for the Turkpin dealer (bayi) API: e-pin games, products,
// orders and balance. Documentation: https://dev.turkpin.com
//
// Every call is an HTTP POST to api.php with one form field, DATA, holding
// <APIRequest><params>...</params></APIRequest>. Responses are XML.
// Money values are returned as decimal strings (e.g. "0.0010") to keep them exact.
package turkpin

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	LiveURL = "https://www.turkpin.com/api.php"
	TestURL = "https://www.turkpin.net/api.php"
)

// Values of SIPARIS_DURUMU returned by siparisDurumu and siparisListesi.
const (
	StatusProcessing         = 1
	StatusCompleted          = 2
	StatusCancelled          = 3
	StatusDelivering         = 99
	StatusPreOrderDelivering = 199
)

// Error is returned when the API answers with an error code other than "000",
// or when the HTTP request / XML response fails (Code is then empty).
type Error struct {
	Code    string
	Message string
}

func (e *Error) Error() string {
	if e.Code == "" {
		return "turkpin: " + e.Message
	}
	return fmt.Sprintf("turkpin: %s (%s)", e.Message, e.Code)
}

type Game struct {
	ID   int
	Name string
}

type Product struct {
	ID       int
	Name     string
	Stock    int
	MinOrder int
	MaxOrder int // 0 means no upper limit
	Price    string
	TaxType  string
	PreOrder bool
	// Tiered (barem) products only; empty otherwise.
	MinBarem  string
	MaxBarem  string
	BaremStep string
}

// Tiered reports whether the product is ordered by amount (barem).
func (p Product) Tiered() bool { return p.MinBarem != "" }

type Epin struct {
	Code        string
	Description string
	ID          string
}

type Order struct {
	OrderNo string
	Result  string // "Success" or "Pending"
	Amount  string
	Epins   []Epin
}

// Pending is true for pre-order / tiered orders: poll OrderStatus for the codes.
func (o Order) Pending() bool { return o.Result != "Success" }

type OrderInfo struct {
	OrderNo     string
	Status      int // one of the Status* constants
	Description string
	StateCode   string // "000" normally, "999" when Turkpin is handling a problem with the order
	CheckedAt   string
	Amount      string
	Extra       string // cancellation reason or problem note
	Epins       []Epin
}

type Balance struct {
	Balance  string
	Credit   string
	Bonus    string
	Spending string
}

// OrderRequest describes an e-pin order. PreOrder must be true for products listed with
// PreOrder; Barem is the amount for tiered products (between MinBarem and MaxBarem, in BaremStep steps).
type OrderRequest struct {
	GameID    int
	ProductID int
	Quantity  int // defaults to 1
	Character string
	PreOrder  bool
	Barem     string
}

type Client struct {
	Username   string
	Password   string
	BaseURL    string
	HTTPClient *http.Client
}

// New returns a client for the live system, or the sandbox when test is true.
func New(username, password string, test bool) *Client {
	u := LiveURL
	if test {
		u = TestURL
	}
	return &Client{Username: username, Password: password, BaseURL: u, HTTPClient: &http.Client{Timeout: 30 * time.Second}}
}

type field struct {
	XMLName xml.Name
	Value   string `xml:",chardata"`
}

// Param is one extra request parameter for Call.
type Param struct{ Key, Value string }

// Params is the generic <params> element of a response.
type Params struct {
	Error        string     `xml:"error"`
	ErrorDesc    string     `xml:"error_desc"`
	HataNo       string     `xml:"HATA_NO"`
	HataAciklama string     `xml:"HATA_ACIKLAMA"`
	Games        []xGame    `xml:"oyunListesi>oyun"`
	Products     []xProduct `xml:"epinUrunListesi>urun"`
	OrderResult  string     `xml:"siparisSonuc"`
	OrderNo      string     `xml:"siparisNo"`
	OrderAmount  string     `xml:"siparisTutari"`
	Epins        []xEpin    `xml:"epin_list>epin"`
	xOrderInfo
	Orders  []xOrderInfo `xml:"SIPARISLER>SIPARIS"`
	Balance *xBalance    `xml:"balanceInformation"`
}

type xBalance struct {
	Balance  string `xml:"balance"`
	Credit   string `xml:"credit"`
	Bonus    string `xml:"bonus"`
	Spending string `xml:"spending"`
}

type xGame struct {
	ID   string `xml:"id"`
	Name string `xml:"name"`
}

type xProduct struct {
	ID        string `xml:"id"`
	Name      string `xml:"name"`
	Stock     string `xml:"stock"`
	MinOrder  string `xml:"min_order"`
	MaxOrder  string `xml:"max_order"`
	Price     string `xml:"price"`
	TaxType   string `xml:"tax_type"`
	PreOrder  string `xml:"pre_order"`
	MinBarem  string `xml:"min_barem"`
	MaxBarem  string `xml:"max_barem"`
	BaremStep string `xml:"barem_step"`
}

type xEpin struct {
	Code string `xml:"code"`
	Desc string `xml:"desc"`
	ID   string `xml:"id"`
}

type xOrderInfo struct {
	StateCode   string  `xml:"DURUM_KODU"`
	No          string  `xml:"SIPARIS_NO"`
	Status      string  `xml:"SIPARIS_DURUMU"`
	CheckedAt   string  `xml:"KONTROL_TARIHI"`
	Description string  `xml:"SIPARIS_DURUMU_ACIKLAMA"`
	Amount      string  `xml:"SIPARIS_TUTARI"`
	Ekstra      string  `xml:"EKSTRA"`
	Extra       string  `xml:"EXTRA"`
	Epins       []xEpin `xml:"epin_list>epin"`
}

func (c *Client) Games(ctx context.Context) ([]Game, error) {
	p, err := c.Call(ctx, "epinOyunListesi")
	if err != nil {
		return nil, err
	}
	out := make([]Game, 0, len(p.Games))
	for _, g := range p.Games {
		out = append(out, Game{ID: atoi(g.ID), Name: strings.TrimSpace(g.Name)})
	}
	return out, nil
}

// Products lists e-pin products of a game; productID 0 lists all of them.
func (c *Client) Products(ctx context.Context, gameID, productID int) ([]Product, error) {
	args := []Param{{"oyunKodu", strconv.Itoa(gameID)}}
	if productID != 0 {
		args = append(args, Param{"urunKodu", strconv.Itoa(productID)})
	}
	p, err := c.Call(ctx, "epinUrunleri", args...)
	if err != nil {
		return nil, err
	}
	out := make([]Product, 0, len(p.Products))
	for _, u := range p.Products {
		minOrder := atoi(u.MinOrder)
		if minOrder < 1 {
			minOrder = 1
		}
		out = append(out, Product{
			ID: atoi(u.ID), Name: strings.TrimSpace(u.Name), Stock: atoi(u.Stock), MinOrder: minOrder,
			MaxOrder: atoi(u.MaxOrder), Price: strings.TrimSpace(u.Price), TaxType: strings.TrimSpace(u.TaxType),
			PreOrder: strings.EqualFold(strings.TrimSpace(u.PreOrder), "true"),
			MinBarem: strings.TrimSpace(u.MinBarem), MaxBarem: strings.TrimSpace(u.MaxBarem),
			BaremStep: strings.TrimSpace(u.BaremStep),
		})
	}
	return out, nil
}

func (c *Client) CreateOrder(ctx context.Context, r OrderRequest) (*Order, error) {
	q := r.Quantity
	if q == 0 {
		q = 1
	}
	args := []Param{{"oyunKodu", strconv.Itoa(r.GameID)}, {"urunKodu", strconv.Itoa(r.ProductID)}, {"adet", strconv.Itoa(q)}}
	if r.Character != "" {
		args = append(args, Param{"character", r.Character})
	}
	if r.PreOrder {
		args = append(args, Param{"pre_order", "true"})
	}
	if r.Barem != "" {
		args = append(args, Param{"barem", r.Barem})
	}
	p, err := c.Call(ctx, "epinSiparisYarat", args...)
	if err != nil {
		return nil, err
	}
	return &Order{OrderNo: strings.TrimSpace(p.OrderNo), Result: strings.TrimSpace(p.OrderResult),
		Amount: strings.TrimSpace(p.OrderAmount), Epins: epins(p.Epins)}, nil
}

func (c *Client) OrderStatus(ctx context.Context, orderNo string) (*OrderInfo, error) {
	p, err := c.Call(ctx, "siparisDurumu", Param{"siparisNo", orderNo})
	if err != nil {
		return nil, err
	}
	o := orderInfo(p.xOrderInfo)
	o.Epins = epins(p.Epins) // epin_list is decoded into Params.Epins (shared path)
	return &o, nil
}

// Orders lists orders between two dates (inclusive, at most 30 days apart).
func (c *Client) Orders(ctx context.Context, start, end time.Time) ([]OrderInfo, error) {
	p, err := c.Call(ctx, "siparisListesi", Param{"baslangicTarihi", start.Format("2006-01-02")}, Param{"bitisTarihi", end.Format("2006-01-02")})
	if err != nil {
		return nil, err
	}
	out := make([]OrderInfo, 0, len(p.Orders))
	for _, s := range p.Orders {
		out = append(out, orderInfo(s))
	}
	return out, nil
}

func (c *Client) Balance(ctx context.Context) (*Balance, error) {
	p, err := c.Call(ctx, "balance")
	if err != nil {
		return nil, err
	}
	if p.Balance == nil {
		return nil, &Error{Message: "balanceInformation missing in response"}
	}
	b := p.Balance
	return &Balance{Balance: strings.TrimSpace(b.Balance), Credit: strings.TrimSpace(b.Credit),
		Bonus: strings.TrimSpace(b.Bonus), Spending: strings.TrimSpace(b.Spending)}, nil
}

// Call sends one command with extra parameters and returns the decoded <params> element.
func (c *Client) Call(ctx context.Context, cmd string, args ...Param) (*Params, error) {
	var buf bytes.Buffer
	buf.WriteString("<APIRequest><params>")
	for _, a := range append([]Param{{"cmd", cmd}, {"username", c.Username}, {"password", c.Password}}, args...) {
		if err := xml.NewEncoder(&buf).Encode(field{XMLName: xml.Name{Local: a.Key}, Value: a.Value}); err != nil {
			return nil, &Error{Message: err.Error()}
		}
	}
	buf.WriteString("</params></APIRequest>")
	form := url.Values{"DATA": {buf.String()}}.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL, strings.NewReader(form))
	if err != nil {
		return nil, &Error{Message: err.Error()}
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	hc := c.HTTPClient
	if hc == nil {
		hc = http.DefaultClient
	}
	res, err := hc.Do(req)
	if err != nil {
		return nil, &Error{Message: "HTTP request failed: " + err.Error()}
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, &Error{Message: "HTTP request failed: " + err.Error()}
	}
	if res.StatusCode >= 400 && res.StatusCode != http.StatusUnauthorized {
		return nil, &Error{Message: fmt.Sprintf("HTTP %d from Turkpin API", res.StatusCode)}
	}
	var doc struct {
		XMLName xml.Name `xml:"APIResponse"`
		Params  *Params  `xml:"params"`
	}
	if err := xml.Unmarshal(bytes.TrimSpace(raw), &doc); err != nil || doc.Params == nil {
		snippet := string(raw)
		if len(snippet) > 200 {
			snippet = snippet[:200]
		}
		return nil, &Error{Message: "invalid response from Turkpin API: " + snippet}
	}
	p := doc.Params
	code := strings.TrimSpace(p.Error)
	if code == "" {
		code = strings.TrimSpace(p.HataNo)
	}
	if code != "" && code != "000" {
		msg := strings.TrimSpace(p.ErrorDesc)
		if msg == "" {
			msg = strings.TrimSpace(p.HataAciklama)
		}
		if msg == "" {
			msg = "Turkpin API error"
		}
		return nil, &Error{Code: code, Message: msg}
	}
	return p, nil
}

func atoi(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}

func epins(in []xEpin) []Epin {
	out := make([]Epin, 0, len(in))
	for _, e := range in {
		out = append(out, Epin{Code: strings.TrimSpace(e.Code), Description: strings.TrimSpace(e.Desc), ID: strings.TrimSpace(e.ID)})
	}
	return out
}

func orderInfo(s xOrderInfo) OrderInfo {
	extra := strings.TrimSpace(s.Ekstra)
	if extra == "" {
		extra = strings.TrimSpace(s.Extra)
	}
	return OrderInfo{OrderNo: strings.TrimSpace(s.No), Status: atoi(s.Status), Description: strings.TrimSpace(s.Description),
		StateCode: strings.TrimSpace(s.StateCode), CheckedAt: strings.TrimSpace(s.CheckedAt),
		Amount: strings.TrimSpace(s.Amount), Extra: extra, Epins: epins(s.Epins)}
}
