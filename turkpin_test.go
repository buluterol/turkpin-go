package turkpin

import (
	"context"
	"encoding/xml"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"
)

type sentParams map[string]string

func setup(t *testing.T, fixture string) (*Client, *sentParams) {
	t.Helper()
	body := []byte(fixture)
	if fixture[0] != '<' {
		var err error
		if body, err = os.ReadFile("testdata/" + fixture); err != nil {
			t.Fatal(err)
		}
	}
	got := &sentParams{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		form, _ := url.ParseQuery(string(raw))
		var doc struct {
			Params struct {
				Fields []struct {
					XMLName xml.Name
					Value   string `xml:",chardata"`
				} `xml:",any"`
			} `xml:"params"`
		}
		if err := xml.Unmarshal([]byte(form.Get("DATA")), &doc); err != nil {
			t.Errorf("bad request XML: %v", err)
		}
		*got = sentParams{"_method": r.Method}
		for _, f := range doc.Params.Fields {
			(*got)[f.XMLName.Local] = f.Value
		}
		w.Write(body)
	}))
	t.Cleanup(srv.Close)
	c := New("dealer@example.com", "s3cr<et", true)
	c.BaseURL = srv.URL
	return c, got
}

var ctx = context.Background()

func TestURLs(t *testing.T) {
	if New("u", "p", true).BaseURL != TestURL || New("u", "p", false).BaseURL != LiveURL {
		t.Fatal("wrong base URLs")
	}
}

func TestRequestFormat(t *testing.T) {
	c, p := setup(t, "games.xml")
	if _, err := c.Games(ctx); err != nil {
		t.Fatal(err)
	}
	if (*p)["_method"] != "POST" || (*p)["cmd"] != "epinOyunListesi" || (*p)["password"] != "s3cr<et" {
		t.Fatalf("unexpected request: %v", *p)
	}
}

func TestGames(t *testing.T) {
	c, _ := setup(t, "games.xml")
	g, err := c.Games(ctx)
	if err != nil || len(g) != 2 || g[0] != (Game{1, "Game 1"}) {
		t.Fatalf("got %v %v", g, err)
	}
}

func TestProducts(t *testing.T) {
	c, p := setup(t, "products.xml")
	list, err := c.Products(ctx, 1, 0)
	if err != nil || len(list) != 2 {
		t.Fatalf("got %v %v", list, err)
	}
	if _, ok := (*p)["urunKodu"]; ok || (*p)["oyunKodu"] != "1" {
		t.Fatalf("params %v", *p)
	}
	a, b := list[0], list[1]
	if a.Stock != 26278 || a.MaxOrder != 0 || a.Price != "0.001" || a.PreOrder || a.Tiered() {
		t.Fatalf("a = %+v", a)
	}
	if !b.PreOrder || !b.Tiered() || b.MinBarem != "25" || b.MaxBarem != "1250" || b.BaremStep != "0.01" || b.MaxOrder != 5 {
		t.Fatalf("b = %+v", b)
	}
}

func TestCreateOrder(t *testing.T) {
	c, p := setup(t, "order_success.xml")
	o, err := c.CreateOrder(ctx, OrderRequest{GameID: 1, ProductID: 4, Quantity: 2, Character: "Player1"})
	if err != nil {
		t.Fatal(err)
	}
	if (*p)["adet"] != "2" || (*p)["character"] != "Player1" {
		t.Fatalf("params %v", *p)
	}
	if _, ok := (*p)["pre_order"]; ok {
		t.Fatal("pre_order should not be sent")
	}
	if o.OrderNo != "25031013105101" || o.Pending() || len(o.Epins) != 1 || o.Epins[0].Code != "S3XS-EEDY-APAE-62EM" || o.Epins[0].Description != "Test Ürün" {
		t.Fatalf("order %+v", o)
	}
}

func TestPreOrderWithBarem(t *testing.T) {
	c, p := setup(t, "order_pending.xml")
	o, err := c.CreateOrder(ctx, OrderRequest{GameID: 1, ProductID: 4, PreOrder: true, Barem: "30.50"})
	if err != nil {
		t.Fatal(err)
	}
	if (*p)["pre_order"] != "true" || (*p)["barem"] != "30.50" || (*p)["adet"] != "1" {
		t.Fatalf("params %v", *p)
	}
	if !o.Pending() || len(o.Epins) != 0 {
		t.Fatalf("order %+v", o)
	}
}

func TestAPIErrors(t *testing.T) {
	c, _ := setup(t, "order_out_of_stock.xml")
	_, err := c.CreateOrder(ctx, OrderRequest{GameID: 1, ProductID: 2})
	var e *Error
	if !errors.As(err, &e) || e.Code != "012" || e.Message != "Urun stok yetersiz." {
		t.Fatalf("err = %v", err)
	}
	c, _ = setup(t, "error_auth.xml")
	if _, err := c.Games(ctx); !errors.As(err, &e) || e.Code != "001" {
		t.Fatalf("err = %v", err)
	}
}

func TestInvalidResponse(t *testing.T) {
	c, _ := setup(t, "<html>blocked</html")
	var e *Error
	if _, err := c.Balance(ctx); !errors.As(err, &e) || e.Code != "" {
		t.Fatalf("err = %v", err)
	}
}

func TestOrderStatus(t *testing.T) {
	c, p := setup(t, "status_completed.xml")
	s, err := c.OrderStatus(ctx, "25031013105101")
	if err != nil || (*p)["siparisNo"] != "25031013105101" {
		t.Fatal(err)
	}
	if s.Status != StatusCompleted || s.Amount != "0.0010" || len(s.Epins) != 1 {
		t.Fatalf("%+v", s)
	}
	c, _ = setup(t, "status_cancelled.xml")
	s, err = c.OrderStatus(ctx, "25030620073001")
	if err != nil || s.Status != StatusCancelled || s.Extra != "Stok yok" {
		t.Fatalf("%+v %v", s, err)
	}
}

func TestOrders(t *testing.T) {
	c, p := setup(t, "orders.xml")
	list, err := c.Orders(ctx, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 1, 30, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if (*p)["baslangicTarihi"] != "2026-01-01" || (*p)["bitisTarihi"] != "2026-01-30" {
		t.Fatalf("params %v", *p)
	}
	if len(list) != 2 || list[0].Status != StatusProcessing || list[1].StateCode != "999" || list[1].Extra != "CANCELLATION REASON" {
		t.Fatalf("%+v", list)
	}
}

func TestBalance(t *testing.T) {
	c, _ := setup(t, "balance.xml")
	b, err := c.Balance(ctx)
	if err != nil || *b != (Balance{"98689.0000", "0.0000", "0.000", "1311.0000"}) {
		t.Fatalf("%+v %v", b, err)
	}
}
