package save

import (
	"errors"
	"log/slog"
	"net/http"
	resp "url-shortener/internal/lib/api/response"
	"url-shortener/internal/lib/logger/sl"
	"url-shortener/internal/lib/random"
	"url-shortener/internal/storage"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/render"
	"github.com/go-playground/validator/v10"
)

type Request struct {
	URL   string `json:"url" validate:"required,url"`
	Alias string `json:"alias,omitempty"`
}

type Response struct {
	resp.Response
	Alias string `json:"alias,omitempty"`
}

const (
	aliasLength = 6
	maxRetries  = 5
)


type URLSaver interface {
	SaveURL(urlToSave string, alias string) (int64, error)
}

func New(log *slog.Logger, urlSaver URLSaver) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const op = "handlers.url.save.New"

		log = log.With(
			slog.String("op", op),
			slog.String("request_id", middleware.GetReqID(r.Context())),
		)

		var req Request
		err := render.DecodeJSON(r.Body, &req)
		if err != nil {
			log.Error("failed to decode request body", sl.Err(err))

			render.JSON(w, r, resp.Error("failed to decode request"))

			return
		}

		log.Info("request body decoded", slog.Any("request", req))

		if err := validator.New().Struct(req); err != nil {
			validateErr := err.(validator.ValidationErrors)

			log.Error("invalide request", sl.Err(err))

			render.JSON(w, r, resp.ValidationError(validateErr))

			return
		}

		alias := req.Alias
		if alias == "" {
			// Генерируем уникальный алиас с повторными попытками
			for i := 0; i < maxRetries; i++ {
				alias = random.NewRandomString(aliasLength)

				id, err := urlSaver.SaveURL(req.URL, alias)
				if err == nil {
					// Успешно сохранили
					log.Info("url added", slog.String("alias", alias), slog.Int64("id", id))

					render.JSON(w, r, Response{
						Response: resp.OK(),
						Alias:    alias,
					})
					return
				}

				if !errors.Is(err, storage.ErrUrlExists) {
					// Другая ошибка, не коллизия
					log.Error("failed to save url", sl.Err(err))
					render.JSON(w, r, resp.Error("failed to save url"))
					return
				}

				// Коллизия алиаса, пробуем снова
				log.Info("alias collision, retrying", slog.String("alias", alias), slog.Int("attempt", i+1))
			}

			// Не удалось сгенерировать уникальный алиас за maxRetries попыток
			log.Error("failed to generate unique alias after retries", slog.Int("max_retries", maxRetries))
			render.JSON(w, r, resp.Error("failed to generate unique alias"))
			return
		}

		// Пользователь предоставил свой алиас
		id, err := urlSaver.SaveURL(req.URL, alias)
		if errors.Is(err, storage.ErrUrlExists) {
			log.Info("url already exists", slog.String("url", req.URL))
			render.JSON(w, r, resp.Error("url already exists"))
			return
		}

		if err != nil {
			log.Error("failed to save url", sl.Err(err))
			render.JSON(w, r, resp.Error("failed to save url"))

			return
		}

		log.Info("url added", slog.String("alias", alias), slog.Int64("id", id))

		render.JSON(w, r, Response{
			Response: resp.OK(),
			Alias:    alias,
		})
	}
}
