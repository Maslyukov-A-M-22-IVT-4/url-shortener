package tests

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/brianvoe/gofakeit/v6"
	"github.com/gavv/httpexpect/v2"
	"github.com/stretchr/testify/require"

	"url-shortener/internal/http-server/handlers/url/save"
	"url-shortener/internal/lib/api"
	"url-shortener/internal/lib/random"
)

const (
	host = "localhost:8082"
)

func TestURLShortener_HappyPath(t *testing.T) {
	u := &url.URL{
		Scheme: "http",
		Host:   host,
	}

	e := httpexpect.Default(t, u.String())

	e.POST("/url").
		WithJSON(save.Request{
			URL:   gofakeit.URL(),
			Alias: random.NewRandomString(10),
		}).
		WithBasicAuth("myuser", "mypass").
		Expect().
		Status(200).
		JSON().
		Path("$.alias").String().NotEmpty()
}

func TestURLShortener_SaveURL(t *testing.T) {
	u := &url.URL{
		Scheme: "http",
		Host:   host,
	}

	e := httpexpect.Default(t, u.String())

	testCases := []struct {
		name   string
		url    string
		alias  string
		status int
	}{
		{
			name:   "Valid URL with custom alias",
			url:    "https://google.com",
			alias:  random.NewRandomString(6),
			status: http.StatusOK,
		},
		{
			name:   "Valid URL without alias",
			url:    "https://github.com",
			alias:  "",
			status: http.StatusOK,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			resp := e.POST("/url").
				WithJSON(save.Request{
					URL:   tc.url,
					Alias: tc.alias,
				}).
				WithBasicAuth("myuser", "mypass").
				Expect().
				Status(tc.status).
				JSON()

			resp.Path("$.status").String().IsEqual("OK")
			resp.Path("$.alias").String().NotEmpty()
		})
	}
}

func TestURLShortener_GetURL(t *testing.T) {
	u := &url.URL{
		Scheme: "http",
		Host:   host,
	}

	// Создаем HTTP клиент без автоматического следования редиректам
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	e := httpexpect.WithConfig(httpexpect.Config{
		BaseURL:  u.String(),
		Client:   client,
		Reporter: httpexpect.NewAssertReporter(t),
		Printers: []httpexpect.Printer{
			httpexpect.NewDebugPrinter(t, true),
		},
	})

	// Сначала создаем URL
	testURL := "https://example.com"
	testAlias := random.NewRandomString(8)

	e.POST("/url").
		WithJSON(save.Request{
			URL:   testURL,
			Alias: testAlias,
		}).
		WithBasicAuth("myuser", "mypass").
		Expect().
		Status(http.StatusOK)

	// Тестируем получение URL
	testCases := []struct {
		name   string
		alias  string
		status int
		url    string
	}{
		{
			name:   "Existing alias",
			alias:  testAlias,
			status: http.StatusFound,
			url:    testURL,
		},
		{
			name:   "Non-existing alias",
			alias:  "nonexistent",
			status: http.StatusOK, // JSON response with error
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			resp := e.GET("/" + tc.alias).Expect()

			if tc.status == http.StatusFound {
				resp.Status(tc.status).
					Header("Location").IsEqual(tc.url)
			} else {
				resp.Status(tc.status).
					JSON().Path("$.error").String().NotEmpty()
			}
		})
	}
}

func TestURLShortener_DuplicateAlias(t *testing.T) {
	u := &url.URL{
		Scheme: "http",
		Host:   host,
	}

	e := httpexpect.Default(t, u.String())

	testURL := "https://duplicate-test.com"
	testAlias := random.NewRandomString(8)

	// Создаем первый URL
	e.POST("/url").
		WithJSON(save.Request{
			URL:   testURL,
			Alias: testAlias,
		}).
		WithBasicAuth("myuser", "mypass").
		Expect().
		Status(http.StatusOK)

	// Пытаемся создать с тем же alias
	e.POST("/url").
		WithJSON(save.Request{
			URL:   "https://another-url.com",
			Alias: testAlias,
		}).
		WithBasicAuth("myuser", "mypass").
		Expect().
		Status(http.StatusOK).
		JSON().Path("$.error").String().Contains("already exists")
}

func TestURLShortener_RandomAlias(t *testing.T) {
	u := &url.URL{
		Scheme: "http",
		Host:   host,
	}

	// Создаем HTTP клиент без автоматического следования редиректам
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	e := httpexpect.WithConfig(httpexpect.Config{
		BaseURL:  u.String(),
		Client:   client,
		Reporter: httpexpect.NewAssertReporter(t),
		Printers: []httpexpect.Printer{
			httpexpect.NewDebugPrinter(t, true),
		},
	})

	testURL := "https://example.com"

	// Создаем URL без указания alias
	resp := e.POST("/url").
		WithJSON(save.Request{
			URL: testURL,
		}).
		WithBasicAuth("myuser", "mypass").
		Expect().
		Status(http.StatusOK).
		JSON()

	// Проверяем, что alias сгенерирован
	alias := resp.Path("$.alias").String().NotEmpty().Raw()

	// Проверяем, что редирект работает
	e.GET("/" + alias).
		Expect().
		Status(http.StatusFound).
		Header("Location").IsEqual(testURL)
}

func TestURLShortener_RedirectFlow(t *testing.T) {
	u := &url.URL{
		Scheme: "http",
		Host:   host,
	}

	e := httpexpect.Default(t, u.String())

	testURL := "https://example.com"

	// Создаем короткую ссылку
	resp := e.POST("/url").
		WithJSON(save.Request{
			URL: testURL,
		}).
		WithBasicAuth("myuser", "mypass").
		Expect().
		Status(http.StatusOK).
		JSON()

	alias := resp.Path("$.alias").String().NotEmpty().Raw()

	// Тестируем редирект через api.GetRedirect
	redirectURL, err := api.GetRedirect("http://" + host + "/" + alias)
	require.NoError(t, err)
	require.Equal(t, testURL, redirectURL)
}

func TestURLShortener_CollisionHandling(t *testing.T) {
	u := &url.URL{
		Scheme: "http",
		Host:   host,
	}

	// Создаем HTTP клиент без автоматического следования редиректам
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	e := httpexpect.WithConfig(httpexpect.Config{
		BaseURL:  u.String(),
		Client:   client,
		Reporter: httpexpect.NewAssertReporter(t),
		Printers: []httpexpect.Printer{
			httpexpect.NewDebugPrinter(t, true),
		},
	})

	// Создаем несколько URL без alias для проверки обработки коллизий
	testURLs := []string{
		"https://example.com/1",
		"https://example.com/2",
		"https://example.com/3",
	}

	for _, testURL := range testURLs {
		resp := e.POST("/url").
			WithJSON(save.Request{
				URL: testURL,
			}).
			WithBasicAuth("myuser", "mypass").
			Expect().
			Status(http.StatusOK).
			JSON()

		// Проверяем, что каждый раз генерируется уникальный alias
		alias := resp.Path("$.alias").String().NotEmpty().Raw()
		require.NotEmpty(t, alias)

		// Проверяем, что редирект работает
		e.GET("/" + alias).
			Expect().
			Status(http.StatusFound).
			Header("Location").IsEqual(testURL)
	}
}

func TestURLShortener_Authentication(t *testing.T) {
	u := &url.URL{
		Scheme: "http",
		Host:   host,
	}

	e := httpexpect.Default(t, u.String())

	testURL := "https://auth-test.com"

	// Тест без аутентификации
	e.POST("/url").
		WithJSON(save.Request{
			URL: testURL,
		}).
		Expect().
		Status(http.StatusUnauthorized)

	// Тест с неправильными данными
	e.POST("/url").
		WithJSON(save.Request{
			URL: testURL,
		}).
		WithBasicAuth("wrong", "credentials").
		Expect().
		Status(http.StatusUnauthorized)

	// Тест с правильными данными
	e.POST("/url").
		WithJSON(save.Request{
			URL: testURL,
		}).
		WithBasicAuth("myuser", "mypass").
		Expect().
		Status(http.StatusOK)
}
