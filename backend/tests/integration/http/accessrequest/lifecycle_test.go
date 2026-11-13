package accessrequest_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	requestservice "github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/accessrequest"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/models"
	"github.com/stretchr/testify/require"
)

func TestHTTPAccessRequestLifecycleAndPermissions(t *testing.T) {
	s, db := integrationService(t)
	router := requestRouter(s)
	ctx := context.Background()
	// Подготовку вызывает серверный инструмент агента, публичного POST для неё нет.
	preparation, err := s.PrepareAccessRequest(ctx, "intern", "repo")
	require.NoError(t, err)
	require.NotNil(t, preparation.Draft)
	confirmPath := requestsPath + "/drafts/" + preparation.Draft.ID + "/confirm"
	checkAPIError(t, send(router, http.MethodPost, confirmPath, `{"confirmed":true}`, "other-intern"), 404, "draft_not_found")
	checkAPIError(t, send(router, http.MethodPost, confirmPath, `{"confirmed":true}`, "manager"), 403, "forbidden")
	w := send(router, http.MethodPost, confirmPath, `{"confirmed":true}`, "intern")
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var request requestservice.Request
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &request))
	require.Equal(t, models.AccessRequestPending, request.Status)
	path := requestsPath + "/" + request.ID
	require.Equal(t, path, w.Header().Get("Location"))
	w = send(router, http.MethodPost, confirmPath, `{"confirmed":true}`, "intern")
	require.Equal(t, http.StatusOK, w.Code)
	var repeated requestservice.Request
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &repeated))
	require.Equal(t, request.ID, repeated.ID)
	for _, actor := range []string{"other-intern", "other-manager"} {
		checkAPIError(t, send(router, http.MethodGet, path, "", actor), 404, "request_not_found")
	}
	checkAPIError(t, send(router, http.MethodPost, path+"/decision", `{"decision":"approve"}`, "intern"), 403, "forbidden")
	checkAPIError(t, send(router, http.MethodPost, path+"/decision", `{"decision":"approve"}`, "other-manager"), 404, "request_not_found")
	checkAPIError(t, send(router, http.MethodPost, path+"/decision", `{"decision":"reject","reason":"  "}`, "manager"), 400, "invalid_input")
	checkAPIError(t, send(router, http.MethodPost, path+"/retry", "", "manager"), 409, "invalid_request_state")
	for _, actor := range []string{"intern", "manager"} {
		w = send(router, http.MethodGet, requestsPath, "", actor)
		require.Equal(t, http.StatusOK, w.Code)
		var list requestservice.RequestList
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &list))
		require.Len(t, list.Items, 1)
		require.EqualValues(t, 1, list.Counts.Pending)
	}
	w = send(router, http.MethodPost, path+"/decision", `{"decision":"approve"}`, "manager")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	w = send(router, http.MethodPost, path+"/decision", `{"decision":"approve"}`, "manager")
	require.Equal(t, http.StatusOK, w.Code)
	checkAPIError(t, send(router, http.MethodPost, path+"/decision", `{"decision":"reject","reason":"Отказ"}`, "manager"), 409, "request_already_decided")
	var jobs int64
	require.NoError(t, db.Model(&models.AccessIssuanceJob{}).Count(&jobs).Error)
	require.EqualValues(t, 1, jobs)
	w = send(router, http.MethodGet, requestsPath+"?group=completed&status=granted&resourceId=missing", "", "manager")
	require.Equal(t, http.StatusOK, w.Code)
	var queue requestservice.RequestList
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &queue))
	require.Empty(t, queue.Items)
	require.Zero(t, queue.Counts.Total, "после одобрения заявка ушла из очереди руководителя")

	provider := &testProvider{failure: errors.New("provider offline")}
	worked, err := s.ProcessNextIssuance(ctx, provider)
	require.True(t, worked)
	require.Error(t, err)
	w = send(router, http.MethodGet, path, "", "manager")
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &request))
	require.Equal(t, models.AccessRequestFailed, request.Status)
	checkAPIError(t, send(router, http.MethodPost, path+"/retry", "", "intern"), 403, "forbidden")
	checkAPIError(t, send(router, http.MethodPost, path+"/retry", "", "other-manager"), 404, "request_not_found")
	w = send(router, http.MethodPost, path+"/retry", "", "manager")
	require.Equal(t, http.StatusOK, w.Code)
	provider.failure = nil
	worked, err = s.ProcessNextIssuance(ctx, provider)
	require.NoError(t, err)
	require.True(t, worked)
	w = send(router, http.MethodGet, path, "", "intern")
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &request))
	require.Equal(t, models.AccessRequestGranted, request.Status)
	require.NotNil(t, request.GrantedAt)
	w = send(router, http.MethodGet, requestsPath+"?group=completed&status=granted", "", "intern")
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &queue))
	require.Len(t, queue.Items, 1)
	require.EqualValues(t, 1, queue.Counts.Granted)
}
