package onec

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"

	"rms/internal/domain/models"
)

type returnLine struct {
	ProductID uuid.UUID `json:"product_id"`
	Quantity  float64   `json:"quantity"`
	Mark      string    `json:"mark"`
}

func (s *Service) SendReturn(ctx context.Context, r models.ReturnRequest, statusOnly bool) (models.ReturnResult, error) {
	endpoint := "ReturnRequestStatus"
	status := struct {
		ID     uuid.UUID           `json:"returnId"`
		Status models.ReturnStatus `json:"status"`
	}{r.ID, r.Status}
	var body any = status
	if !statusOnly {
		endpoint = "ReturnRequest"
		items := make([]returnLine, 0, len(r.Items))
		for _, item := range r.Items {
			if len(item.Marks) == 0 {
				items = append(items, returnLine{item.ProductID, item.Amount, ""})
			}
			for _, mark := range item.Marks {
				items = append(items, returnLine{item.ProductID, 1, mark})
			}
		}
		body = struct {
			ID     uuid.UUID           `json:"returnId"`
			Date   time.Time           `json:"date"`
			Seller uuid.UUID           `json:"seller"`
			Store  uuid.UUID           `json:"storeId"`
			Status models.ReturnStatus `json:"status"`
			Items  []returnLine        `json:"items"`
		}{r.ID, r.Date, r.Seller, r.StoreID, r.Status, items}
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return models.ReturnResult{}, err
	}
	return s.returnResult(ctx, http.MethodPost, "v1/"+endpoint, bytes.NewReader(payload))
}

func (s *Service) GetReturnRequestStatus(ctx context.Context, id uuid.UUID) (models.ReturnStatus, error) {
	query := url.Values{"returnId": {id.String()}}
	result, err := s.returnResult(ctx, http.MethodGet, "v1/GetReturnRequestStatus/?"+query.Encode(), nil)
	if err != nil {
		return "", err
	}
	switch result.Status {
	case models.ReturnNew:
		return models.ReturnProcessed, nil
	case models.ReturnProcessed, models.ReturnCompleted, models.ReturnCancelled:
		return result.Status, nil
	default:
		return "", fmt.Errorf("Неизвестный статус заявки в 1С: %q", result.Status)
	}
}

func (s *Service) ReturnRequestsStatuses(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]models.ReturnStatus, error) {
	payload, err := json.Marshal(struct {
		IDs []uuid.UUID `json:"returnIds"`
	}{IDs: ids})
	if err != nil {
		return nil, err
	}
	var response struct {
		returnResponseMeta
		Data *[]struct {
			ID     uuid.UUID           `json:"returnId"`
			Status models.ReturnStatus `json:"status"`
		} `json:"data"`
	}
	if err := s.readReturnResponse(ctx, http.MethodPost, "v1/ReturnRequestsStatuses", bytes.NewReader(payload), &response, &response.returnResponseMeta); err != nil {
		return nil, err
	}
	if response.Data == nil {
		return nil, errors.New("В ответе 1С отсутствует список статусов заявок")
	}
	requested := make(map[uuid.UUID]struct{}, len(ids))
	for _, id := range ids {
		requested[id] = struct{}{}
	}
	statuses := make(map[uuid.UUID]models.ReturnStatus, len(*response.Data))
	for _, item := range *response.Data {
		if _, ok := requested[item.ID]; !ok {
			return nil, fmt.Errorf("1С вернула статус незапрошенной заявки %s", item.ID)
		}
		status := normalizeReturnStatus(item.Status)
		if !status.Valid() {
			return nil, fmt.Errorf("Неизвестный статус заявки %s в 1С: %q", item.ID, item.Status)
		}
		if previous, exists := statuses[item.ID]; exists && previous != status {
			return nil, fmt.Errorf("1С вернула разные статусы заявки %s", item.ID)
		}
		statuses[item.ID] = status
	}
	return statuses, nil
}

func normalizeReturnStatus(status models.ReturnStatus) models.ReturnStatus {
	if status == "CANCELED" {
		return models.ReturnCancelled
	}
	return status
}

func (s *Service) returnResult(ctx context.Context, method, endpoint string, payload io.Reader) (models.ReturnResult, error) {
	var response struct {
		models.ReturnResult
		returnResponseMeta
		Data *models.ReturnResult `json:"data"`
	}
	if err := s.readReturnResponse(ctx, method, endpoint, payload, &response, &response.returnResponseMeta); err != nil {
		return models.ReturnResult{}, err
	}
	result := response.ReturnResult
	if response.Data != nil {
		result = *response.Data
	}
	result.Status = normalizeReturnStatus(result.Status)
	if !result.Status.Valid() && response.Mess != "" {
		return models.ReturnResult{}, errors.New(response.Mess)
	}
	return result, nil
}

func (s *Service) readReturnResponse(ctx context.Context, method, endpoint string, payload io.Reader, response any, meta *returnResponseMeta) error {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	resp, err := s.do(ctx, method, endpoint, payload)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	decodeErr := json.NewDecoder(io.LimitReader(resp.Body, 16<<20)).Decode(response)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || (meta.Code != nil && *meta.Code != 0) {
		if meta.Mess != "" {
			return errors.New(meta.Mess)
		}
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return fmt.Errorf("1С вернула код %d", *meta.Code)
		}
		return fmt.Errorf("1С вернула HTTP %d", resp.StatusCode)
	}
	if decodeErr != nil {
		return fmt.Errorf("decode 1C %s: %w", endpoint, decodeErr)
	}
	return nil
}

type returnResponseMeta struct {
	Code *int   `json:"code"`
	Mess string `json:"mess"`
}
