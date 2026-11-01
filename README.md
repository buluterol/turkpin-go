# turkpin-go

[![test](https://github.com/buluterol/turkpin-go/actions/workflows/test.yml/badge.svg)](https://github.com/buluterol/turkpin-go/actions/workflows/test.yml)

Turkpin bayi API'si için Go istemcisi: e-pin oyun ve ürün listesi, sipariş verme, sipariş durumu, sipariş listesi ve bakiye.

> Bu paket topluluk tarafından yazılmıştır; Turkpin'in resmi paketi değildir. API'nin güncel ve bağlayıcı tanımı için [dev.turkpin.com](https://dev.turkpin.com) dokümanına bakın.

Diğer diller / Other languages: [PHP](https://github.com/buluterol/turkpin-php) · [Node.js](https://github.com/buluterol/turkpin-node) · [Python](https://github.com/buluterol/turkpin-python) · [Go](https://github.com/buluterol/turkpin-go) · [Java](https://github.com/buluterol/turkpin-java) · [.NET](https://github.com/buluterol/turkpin-dotnet) · [Ruby](https://github.com/buluterol/turkpin-ruby) · [Rust](https://github.com/buluterol/turkpin-rust)

## Gereksinimler

Go 1.21+, bağımlılık yok

## Kurulum

```sh
go get github.com/buluterol/turkpin-go
```



## Kullanım

```go
package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	turkpin "github.com/buluterol/turkpin-go"
)

func main() {
	ctx := context.Background()
	tp := turkpin.New(os.Getenv("TURKPIN_USER"), os.Getenv("TURKPIN_PASS"), true) // true: test ortamı

	games, err := tp.Games(ctx)
	if err != nil {
		panic(err)
	}
	products, _ := tp.Products(ctx, games[0].ID, 0) // 0: tüm ürünler

	order, err := tp.CreateOrder(ctx, turkpin.OrderRequest{GameID: games[0].ID, ProductID: products[0].ID, Quantity: 1})
	var apiErr *turkpin.Error
	if errors.As(err, &apiErr) {
		fmt.Printf("Hata %s: %s\n", apiErr.Code, apiErr.Message)
		return
	}
	if order.Pending() {
		info, _ := tp.OrderStatus(ctx, order.OrderNo) // info.Status == turkpin.StatusCompleted olunca kodlar gelir
		_ = info
	}
	for _, e := range order.Epins {
		fmt.Println(e.Code)
	}

	bal, _ := tp.Balance(ctx)
	fmt.Println(bal.Balance) // tutarlar hassasiyet kaybı olmasın diye string
}
```

## Web framework'leri

İstemci eşzamanlı kullanıma uygundur (`*http.Client` gibi); uygulama açılışında bir kez oluşturup handler'lara verin. İstek bağlamını geçirmek yeterlidir, ek adaptör gerekmez.

```go
tp := turkpin.New(os.Getenv("TURKPIN_USERNAME"), os.Getenv("TURKPIN_PASSWORD"), false)

// net/http, Chi
http.HandleFunc("/balance", func(w http.ResponseWriter, r *http.Request) {
	bal, err := tp.Balance(r.Context())
	// ...
})

// Gin
r.GET("/balance", func(c *gin.Context) { bal, err := tp.Balance(c.Request.Context()) /* ... */ })

// Echo
e.GET("/balance", func(c echo.Context) error { bal, err := tp.Balance(c.Request().Context()) /* ... */ })

// Fiber
app.Get("/balance", func(c *fiber.Ctx) error { bal, err := tp.Balance(c.UserContext()) /* ... */ })
```

## Desteklenen komutlar

| API komutu | Açıklama |
|---|---|
| `epinOyunListesi` | Oyun listesi |
| `epinUrunleri` | Bir oyunun e-pin ürünleri (stok, fiyat, min/max adet, ön sipariş, barem) |
| `epinSiparisYarat` | Sipariş verme (karakter adı, ön sipariş ve barem tutarı isteğe bağlı) |
| `siparisDurumu` | Sipariş durumu ve teslim edilen kodlar |
| `siparisListesi` | İki tarih arasındaki siparişler (en çok 30 gün) |
| `balance` | Bakiye, kredi, bonus ve harcama |

Listede olmayan komutlar için istemcideki genel `call` metodu kullanılabilir.

**Sipariş durumları (`SIPARIS_DURUMU`):** 1 işleniyor, 2 tamamlandı, 3 iptal, 99 teslimatta, 199 ön sipariş teslimatta. `DURUM_KODU` 999 ise Turkpin siparişle ilgili bir sorunu inceliyor demektir; iptal sebebi ya da not `EKSTRA` alanında gelir.

**Ön sipariş ve barem:** Ürünün `pre_order` alanı `true` ise sipariş ön sipariş olarak verilmelidir. Barem tutarlı ürünlerde tutar `min_barem` ile `max_barem` arasında, `barem_step` adımlarıyla seçilir. Bu siparişlerin sonucu `Pending` döner; kodlar için sipariş durumunu sorgulayın.

**Hatalar:** API `000` dışında bir hata kodu döndürdüğünde istemci, kodu ve açıklamayı taşıyan bir hata fırlatır. HTTP ya da XML hatalarında kod boştur.

## Test ortamı

- Test: `https://www.turkpin.net/api.php`, canlı: `https://www.turkpin.com/api.php`.
- Test ortamına erişmek için sunucu IP adresinizin Turkpin'e tanıtılması gerekir; tanıtılmamış IP'den gelen isteklere API `002` hata koduyla cevap verir. Test hesabı ve IP tanımı için [dev.turkpin.com](https://dev.turkpin.com) adresindeki iletişim bilgilerini kullanın.
- Kullanıcı adı ve şifreyi koda yazmayın; ortam değişkeni ya da gizli anahtar deposundan okuyun.

## Geliştirme

Testler gerçek API'ye bağlanmaz; örnek XML cevaplarıyla çalışır.

```sh
go test ./...
```

## English

Go client for the Turkpin dealer API (e-pin games, products, orders, order status, order history, balance). Community-maintained, not an official Turkpin package; see [dev.turkpin.com](https://dev.turkpin.com) for the authoritative API reference. The sandbox (`turkpin.net`) only accepts requests from server IP addresses registered with Turkpin (otherwise error `002`). Install and usage are shown above; method names are in English, while API command and field names follow the original Turkish API.

## Lisans

MIT
