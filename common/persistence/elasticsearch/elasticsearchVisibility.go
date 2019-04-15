// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package elasticsearch

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/olivere/elastic"
	"github.com/pkg/errors"
	"github.com/uber-common/bark"
	workflow "github.com/uber/cadence/.gen/go/shared"
	"github.com/uber/cadence/common"
	es "github.com/uber/cadence/common/elasticsearch"
	"github.com/uber/cadence/common/logging"
	p "github.com/uber/cadence/common/persistence"
	"github.com/uber/cadence/common/service/config"
)

const (
	esPersistenceName = "elasticsearch"
)

type (
	esVisibilityManager struct {
		esClient   es.Client
		index      string
		logger     bark.Logger
		config     *config.VisibilityConfig
		serializer p.CadenceSerializer
	}

	esVisibilityPageToken struct {
		// for ES API From+Size
		From int
		// for ES API searchAfter
		SortTime   int64  // startTime or closeTime
		TieBreaker string // runID
	}

	visibilityRecord struct {
		WorkflowID    string
		RunID         string
		WorkflowType  string
		StartTime     int64
		ExecutionTime int64
		CloseTime     int64
		CloseStatus   workflow.WorkflowExecutionCloseStatus
		HistoryLength int64
		Memo          []byte
		Encoding      string
	}
)

var _ p.VisibilityManager = (*esVisibilityManager)(nil)

var (
	errOperationNotSupported = errors.New("operation not support")

	oneMilliSecondInNano = int64(1000)
)

// NewElasticSearchVisibilityManager create a visibility manager connecting to ElasticSearch
func NewElasticSearchVisibilityManager(esClient es.Client, index string, config *config.VisibilityConfig, logger bark.Logger) p.VisibilityManager {
	return &esVisibilityManager{
		esClient:   esClient,
		index:      index,
		logger:     logger.WithField(logging.TagWorkflowComponent, logging.TagValueESVisibilityManager),
		config:     config,
		serializer: p.NewCadenceSerializer(),
	}
}

func (v *esVisibilityManager) Close() {}

func (v *esVisibilityManager) GetName() string {
	return esPersistenceName
}

func (v *esVisibilityManager) RecordWorkflowExecutionStarted(request *p.RecordWorkflowExecutionStartedRequest) error {
	return errOperationNotSupported
}

func (v *esVisibilityManager) RecordWorkflowExecutionClosed(request *p.RecordWorkflowExecutionClosedRequest) error {
	return errOperationNotSupported
}

func (v *esVisibilityManager) ListOpenWorkflowExecutions(
	request *p.ListWorkflowExecutionsRequest) (*p.ListWorkflowExecutionsResponse, error) {
	token, err := v.getNextPageToken(request.NextPageToken)
	if err != nil {
		return nil, err
	}

	isOpen := true
	searchResult, err := v.getSearchResult(request, token, nil, isOpen)
	if err != nil {
		return nil, &workflow.InternalServiceError{
			Message: fmt.Sprintf("ListOpenWorkflowExecutions failed. Error: %v", err),
		}
	}

	return v.getListWorkflowExecutionsResponse(searchResult.Hits, token, isOpen, request.PageSize)
}

func (v *esVisibilityManager) ListClosedWorkflowExecutions(
	request *p.ListWorkflowExecutionsRequest) (*p.ListWorkflowExecutionsResponse, error) {

	token, err := v.getNextPageToken(request.NextPageToken)
	if err != nil {
		return nil, err
	}

	isOpen := false
	searchResult, err := v.getSearchResult(request, token, nil, isOpen)
	if err != nil {
		return nil, &workflow.InternalServiceError{
			Message: fmt.Sprintf("ListClosedWorkflowExecutions failed. Error: %v", err),
		}
	}

	return v.getListWorkflowExecutionsResponse(searchResult.Hits, token, isOpen, request.PageSize)
}

func (v *esVisibilityManager) ListOpenWorkflowExecutionsByType(
	request *p.ListWorkflowExecutionsByTypeRequest) (*p.ListWorkflowExecutionsResponse, error) {

	token, err := v.getNextPageToken(request.NextPageToken)
	if err != nil {
		return nil, err
	}

	isOpen := true
	matchQuery := elastic.NewMatchQuery(es.WorkflowType, request.WorkflowTypeName)
	searchResult, err := v.getSearchResult(&request.ListWorkflowExecutionsRequest, token, matchQuery, isOpen)
	if err != nil {
		return nil, &workflow.InternalServiceError{
			Message: fmt.Sprintf("ListOpenWorkflowExecutionsByType failed. Error: %v", err),
		}
	}

	return v.getListWorkflowExecutionsResponse(searchResult.Hits, token, isOpen, request.PageSize)
}

func (v *esVisibilityManager) ListClosedWorkflowExecutionsByType(
	request *p.ListWorkflowExecutionsByTypeRequest) (*p.ListWorkflowExecutionsResponse, error) {

	token, err := v.getNextPageToken(request.NextPageToken)
	if err != nil {
		return nil, err
	}

	isOpen := false
	matchQuery := elastic.NewMatchQuery(es.WorkflowType, request.WorkflowTypeName)
	searchResult, err := v.getSearchResult(&request.ListWorkflowExecutionsRequest, token, matchQuery, isOpen)
	if err != nil {
		return nil, &workflow.InternalServiceError{
			Message: fmt.Sprintf("ListClosedWorkflowExecutionsByType failed. Error: %v", err),
		}
	}

	return v.getListWorkflowExecutionsResponse(searchResult.Hits, token, isOpen, request.PageSize)
}

func (v *esVisibilityManager) ListOpenWorkflowExecutionsByWorkflowID(
	request *p.ListWorkflowExecutionsByWorkflowIDRequest) (*p.ListWorkflowExecutionsResponse, error) {

	token, err := v.getNextPageToken(request.NextPageToken)
	if err != nil {
		return nil, err
	}

	isOpen := true
	matchQuery := elastic.NewMatchQuery(es.WorkflowID, request.WorkflowID)
	searchResult, err := v.getSearchResult(&request.ListWorkflowExecutionsRequest, token, matchQuery, isOpen)
	if err != nil {
		return nil, &workflow.InternalServiceError{
			Message: fmt.Sprintf("ListOpenWorkflowExecutionsByWorkflowID failed. Error: %v", err),
		}
	}

	return v.getListWorkflowExecutionsResponse(searchResult.Hits, token, isOpen, request.PageSize)
}

func (v *esVisibilityManager) ListClosedWorkflowExecutionsByWorkflowID(
	request *p.ListWorkflowExecutionsByWorkflowIDRequest) (*p.ListWorkflowExecutionsResponse, error) {

	token, err := v.getNextPageToken(request.NextPageToken)
	if err != nil {
		return nil, err
	}

	isOpen := false
	matchQuery := elastic.NewMatchQuery(es.WorkflowID, request.WorkflowID)
	searchResult, err := v.getSearchResult(&request.ListWorkflowExecutionsRequest, token, matchQuery, isOpen)
	if err != nil {
		return nil, &workflow.InternalServiceError{
			Message: fmt.Sprintf("ListClosedWorkflowExecutionsByWorkflowID failed. Error: %v", err),
		}
	}

	return v.getListWorkflowExecutionsResponse(searchResult.Hits, token, isOpen, request.PageSize)
}

func (v *esVisibilityManager) ListClosedWorkflowExecutionsByStatus(
	request *p.ListClosedWorkflowExecutionsByStatusRequest) (*p.ListWorkflowExecutionsResponse, error) {

	token, err := v.getNextPageToken(request.NextPageToken)
	if err != nil {
		return nil, err
	}

	isOpen := false
	matchQuery := elastic.NewMatchQuery(es.CloseStatus, int32(request.Status))
	searchResult, err := v.getSearchResult(&request.ListWorkflowExecutionsRequest, token, matchQuery, isOpen)
	if err != nil {
		return nil, &workflow.InternalServiceError{
			Message: fmt.Sprintf("ListClosedWorkflowExecutionsByStatus failed. Error: %v", err),
		}
	}

	return v.getListWorkflowExecutionsResponse(searchResult.Hits, token, isOpen, request.PageSize)
}

func (v *esVisibilityManager) GetClosedWorkflowExecution(
	request *p.GetClosedWorkflowExecutionRequest) (*p.GetClosedWorkflowExecutionResponse, error) {

	matchDomainQuery := elastic.NewMatchQuery(es.DomainID, request.DomainUUID)
	existClosedStatusQuery := elastic.NewExistsQuery(es.CloseStatus)
	matchWorkflowIDQuery := elastic.NewMatchQuery(es.WorkflowID, request.Execution.GetWorkflowId())
	boolQuery := elastic.NewBoolQuery().Must(matchDomainQuery).Must(existClosedStatusQuery).Must(matchWorkflowIDQuery)
	rid := request.Execution.GetRunId()
	if rid != "" {
		matchRunIDQuery := elastic.NewMatchQuery(es.RunID, rid)
		boolQuery = boolQuery.Must(matchRunIDQuery)
	}

	ctx := context.Background()
	params := &es.SearchParameters{
		Index: v.index,
		Query: boolQuery,
	}
	searchResult, err := v.esClient.Search(ctx, params)
	if err != nil {
		return nil, &workflow.InternalServiceError{
			Message: fmt.Sprintf("GetClosedWorkflowExecution failed. Error: %v", err),
		}
	}

	response := &p.GetClosedWorkflowExecutionResponse{}
	actualHits := searchResult.Hits.Hits
	if len(actualHits) == 0 {
		return response, nil
	}
	response.Execution = v.convertSearchResultToVisibilityRecord(actualHits[0], false)

	return response, nil
}

func (v *esVisibilityManager) DeleteWorkflowExecution(request *p.VisibilityDeleteWorkflowExecutionRequest) error {
	return nil // not applicable for elastic search, which relies on retention policies for deletion
}

func (v *esVisibilityManager) getNextPageToken(token []byte) (*esVisibilityPageToken, error) {
	var result *esVisibilityPageToken
	var err error
	if len(token) > 0 {
		result, err = v.deserializePageToken(token)
		if err != nil {
			return nil, err
		}
	} else {
		result = &esVisibilityPageToken{}
	}
	return result, nil
}

func (v *esVisibilityManager) getSearchResult(request *p.ListWorkflowExecutionsRequest, token *esVisibilityPageToken,
	matchQuery *elastic.MatchQuery, isOpen bool) (*elastic.SearchResult, error) {

	matchDomainQuery := elastic.NewMatchQuery(es.DomainID, request.DomainUUID)
	existClosedStatusQuery := elastic.NewExistsQuery(es.CloseStatus)
	var rangeQuery *elastic.RangeQuery
	if isOpen {
		rangeQuery = elastic.NewRangeQuery(es.StartTime)
	} else {
		rangeQuery = elastic.NewRangeQuery(es.CloseTime)
	}
	// ElasticSearch v6 is unable to precisely compare time, have to manually add resolution 1ms to time range.
	rangeQuery = rangeQuery.
		Gte(request.EarliestStartTime - oneMilliSecondInNano).
		Lte(request.LatestStartTime + oneMilliSecondInNano)

	boolQuery := elastic.NewBoolQuery().Must(matchDomainQuery).Filter(rangeQuery)
	if matchQuery != nil {
		boolQuery = boolQuery.Must(matchQuery)
	}
	if isOpen {
		boolQuery = boolQuery.MustNot(existClosedStatusQuery)
	} else {
		boolQuery = boolQuery.Must(existClosedStatusQuery)
	}

	ctx := context.Background()
	params := &es.SearchParameters{
		Index:    v.index,
		Query:    boolQuery,
		From:     token.From,
		PageSize: request.PageSize,
	}
	if isOpen {
		params.Sorter = append(params.Sorter, elastic.NewFieldSort(es.StartTime).Desc())
	} else {
		params.Sorter = append(params.Sorter, elastic.NewFieldSort(es.CloseTime).Desc())
	}
	params.Sorter = append(params.Sorter, elastic.NewFieldSort(es.RunID).Desc())

	if token.SortTime != 0 && token.TieBreaker != "" {
		params.SearchAfter = []interface{}{token.SortTime, token.TieBreaker}
	}

	return v.esClient.Search(ctx, params)
}

func (v *esVisibilityManager) getListWorkflowExecutionsResponse(searchHits *elastic.SearchHits,
	token *esVisibilityPageToken, isOpen bool, pageSize int) (*p.ListWorkflowExecutionsResponse, error) {

	response := &p.ListWorkflowExecutionsResponse{}
	actualHits := searchHits.Hits
	numOfActualHits := len(actualHits)

	response.Executions = make([]*workflow.WorkflowExecutionInfo, 0)
	for i := 0; i < numOfActualHits; i++ {
		workflowExecutionInfo := v.convertSearchResultToVisibilityRecord(actualHits[i], isOpen)
		response.Executions = append(response.Executions, workflowExecutionInfo)
	}

	if numOfActualHits == pageSize { // this means the response is not the last page
		var nextPageToken []byte
		var err error

		// ES Search API support pagination using From and PageSize, but has limit that From+PageSize cannot exceed a threshold
		// to retrieve deeper pages, use ES SearchAfter
		if searchHits.TotalHits <= int64(v.config.ESIndexMaxResultWindow()) { // use ES Search From+Size
			nextPageToken, err = v.serializePageToken(&esVisibilityPageToken{From: token.From + numOfActualHits})
		} else { // use ES Search After
			lastExecution := response.Executions[len(response.Executions)-1]
			var sortTime int64
			if isOpen {
				sortTime = lastExecution.GetStartTime()
			} else {
				sortTime = lastExecution.GetCloseTime()
			}
			nextPageToken, err = v.serializePageToken(&esVisibilityPageToken{SortTime: sortTime, TieBreaker: lastExecution.GetExecution().GetRunId()})
		}
		if err != nil {
			return nil, err
		}

		response.NextPageToken = make([]byte, len(nextPageToken))
		copy(response.NextPageToken, nextPageToken)
	}

	return response, nil
}

func (v *esVisibilityManager) deserializePageToken(data []byte) (*esVisibilityPageToken, error) {
	var token esVisibilityPageToken
	err := json.Unmarshal(data, &token)
	if err != nil {
		return nil, &workflow.BadRequestError{
			Message: fmt.Sprintf("unable to deserialize page token. err: %v", err),
		}
	}
	return &token, nil
}

func (v *esVisibilityManager) serializePageToken(token *esVisibilityPageToken) ([]byte, error) {
	data, err := json.Marshal(token)
	if err != nil {
		return nil, &workflow.BadRequestError{
			Message: fmt.Sprintf("unable to serialize page token. err: %v", err),
		}
	}
	return data, nil
}

func (v *esVisibilityManager) convertSearchResultToVisibilityRecord(hit *elastic.SearchHit, isOpen bool) *workflow.WorkflowExecutionInfo {
	var source *visibilityRecord
	err := json.Unmarshal(*hit.Source, &source)
	if err != nil { // log and skip error
		v.logger.WithFields(bark.Fields{
			"error": err.Error(),
			"docID": hit.Id,
		}).Error("unable to unmarshal search hit source")
		return nil
	}

	execution := &workflow.WorkflowExecution{
		WorkflowId: common.StringPtr(source.WorkflowID),
		RunId:      common.StringPtr(source.RunID),
	}
	wfType := &workflow.WorkflowType{
		Name: common.StringPtr(source.WorkflowType),
	}
	if source.ExecutionTime == 0 {
		source.ExecutionTime = source.StartTime
	}

	memo, err := v.serializer.DeserializeVisibilityMemo(p.NewDataBlob(source.Memo, common.EncodingType(source.Encoding)))
	if err != nil {
		v.logger.WithFields(bark.Fields{
			"error": err.Error(),
			"docID": hit.Id,
		}).Error("unable to decode memo field")
	}

	var record *workflow.WorkflowExecutionInfo
	if isOpen {
		record = &workflow.WorkflowExecutionInfo{
			Execution:     execution,
			Type:          wfType,
			StartTime:     common.Int64Ptr(source.StartTime),
			ExecutionTime: common.Int64Ptr(source.ExecutionTime),
			Memo:          memo,
		}
	} else {
		record = &workflow.WorkflowExecutionInfo{
			Execution:     execution,
			Type:          wfType,
			StartTime:     common.Int64Ptr(source.StartTime),
			ExecutionTime: common.Int64Ptr(source.ExecutionTime),
			CloseTime:     common.Int64Ptr(source.CloseTime),
			CloseStatus:   &source.CloseStatus,
			HistoryLength: common.Int64Ptr(source.HistoryLength),
			Memo:          memo,
		}
	}
	return record
}
