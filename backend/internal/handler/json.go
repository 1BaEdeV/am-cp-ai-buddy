package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
)

const maxBodySize = 1 << 20

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// WriteAPIError возвращает ошибку в общем формате API.
func WriteAPIError(w http.ResponseWriter, status int, code, message string) {
	WriteJSON(w, status, apiError{Code: code, Message: message})
}

// WriteJSON записывает JSON-ответ с заданным HTTP-статусом.
func WriteJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		slog.Error("записать JSON-ответ", "error", err)
	}
}

// DecodeJSON разбирает тело запроса. При ошибке сам записывает ответ и возвращает false.
func DecodeJSON(w http.ResponseWriter, r *http.Request, dest any) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		WriteAPIError(w, http.StatusUnsupportedMediaType, "unsupported_media_type", "Передай тело запроса в формате application/json.")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)
	decoder := json.NewDecoder(r.Body)

	var object json.RawMessage
	if err := decoder.Decode(&object); err != nil {
		return writeDecodeError(w, err)
	}
	if string(object) == "null" {
		return writeDecodeError(w, errors.New("ожидался JSON-объект"))
	}
	objectDecoder := json.NewDecoder(bytes.NewReader(object))
	objectDecoder.DisallowUnknownFields()
	if err := objectDecoder.Decode(dest); err != nil {
		return writeDecodeError(w, err)
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return writeDecodeError(w, err)
	}
	return true
}

func writeDecodeError(w http.ResponseWriter, err error) bool {
	var maxBytesError *http.MaxBytesError
	if errors.As(err, &maxBytesError) {
		WriteAPIError(w, http.StatusRequestEntityTooLarge, "request_too_large", "Тело запроса слишком большое.")
	} else {
		WriteAPIError(w, http.StatusBadRequest, "invalid_input", "Передай один JSON-объект с полями из API.")
	}
	return false
}
