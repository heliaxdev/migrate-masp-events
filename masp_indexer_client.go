package main

import (
	"bufio"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/segmentio/encoding/json"
)

const DefaultMaxConcurrentRequests = 100

type maspIndexerHealthResponse struct {
	Commit string `json:"commit"`
}

type MaspIndexerClient struct {
	// URL pointing to https://.../api/v1
	maspIndexerApiV1 string
	// Custom client config
	client http.Client
	// Number of connections in the pool
	maxConcurrentRequests int
}

type TransactionSlot struct {
	MaspTxIndex int    `json:"masp_tx_index"`
	Bytes       []byte `json:"bytes"`
}

type Transaction struct {
	BlockIndex int               `json:"block_index"`
	Batch      []TransactionSlot `json:"batch"`
}

type serverResponse struct {
	Txs []Transaction `json:"txs"`
}

func init() {
	rand.Seed(time.Now().UnixNano())
}

func NewMaspIndexerClient(url string, maxConcurrentRequests int) (*MaspIndexerClient, error) {
	if !strings.HasSuffix(url, "/api/v1") {
		return nil, fmt.Errorf(
			`the url %q does not end with "/api/v1"`,
			url,
		)
	}

	t := http.DefaultTransport.(*http.Transport).Clone()

	// use either HTTP/1 or HTTP/2
	t.Protocols = new(http.Protocols)
	t.Protocols.SetHTTP1(true)
	t.Protocols.SetHTTP2(true)

	client := &MaspIndexerClient{
		maspIndexerApiV1: url,
		client:           http.Client{Transport: t},
	}

	return client, nil
}

func (m *MaspIndexerClient) ValidateVersion(invalidCommitNotErr bool) error {
	health, err := m.health()
	if err != nil {
		return err
	}

	if health.Commit != "38093aca7bc8cd3bb03ef06ce139fe4e672b20ff" {
		if !invalidCommitNotErr {
			return fmt.Errorf(
				"using invalid commit %q, expected 38093aca7bc8cd3bb03ef06ce139fe4e672b20ff (1.2.0)",
				health.Commit,
			)
		}
		if health.Commit == "" {
			health.Commit = "<unknown-commit>"
		}
		log.Println(
			"warning: masp indexer commit does not match release 1.2.0,",
			m.maspIndexerApiV1,
			"is using",
			health.Commit,
		)
	}

	return nil
}

func (m *MaspIndexerClient) health() (*maspIndexerHealthResponse, error) {
	var response maspIndexerHealthResponse

	req, err := http.NewRequest(
		"GET",
		fmt.Sprintf("%s/health", m.maspIndexerApiV1[:len(m.maspIndexerApiV1)-7]),
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to create request to health endpoint from %s: %w",
			m.maspIndexerApiV1,
			err,
		)
	}

	rsp, err := m.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to query health endpoint from %s: %w",
			m.maspIndexerApiV1,
			err,
		)
	}
	defer rsp.Body.Close()

	err = json.NewDecoder(bufio.NewReader(rsp.Body)).Decode(&response)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to decode health endpoint response from %s: %w",
			m.maspIndexerApiV1,
			err,
		)
	}

	return &response, nil
}

func (m *MaspIndexerClient) BlockHeight(height int) (txs []Transaction, err error) {
	const (
		maxJitterMs = 50
		maxRetries  = 10
		maxBackoff  = 1 * time.Second
	)

	randomJitter := func() time.Duration {
		return time.Duration(rand.Intn(maxJitterMs)) * time.Millisecond
	}

	exponentialBackoff := func(retryCounter int) time.Duration {
		return time.Duration(1<<retryCounter) * time.Millisecond
	}

	backoff := func(retryCounter int) time.Duration {
		backoff := exponentialBackoff(retryCounter) + randomJitter()

		if backoff > maxBackoff {
			backoff = maxBackoff
		}

		return backoff
	}

	for i := 0; i < maxRetries; i++ {
		txs, err = m.blockHeight(height)
		if err == nil {
			return
		}
		sleepDuration := backoff(i)
		log.Println(
			"fetch of masp txs of block",
			height,
			"failed, retrying after sleeping for",
			sleepDuration,
		)
		time.Sleep(sleepDuration)
	}

	log.Println("exhausted all", maxRetries, "attempts to fetch block", height)

	return
}

func (m *MaspIndexerClient) blockHeight(height int) ([]Transaction, error) {
	req, err := http.NewRequest(
		"GET",
		fmt.Sprintf(
			"%s/tx?height=%d&height_offset=0",
			m.maspIndexerApiV1,
			height,
		),
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create request to height %d: %w", height, err)
	}

	req.Header.Add(
		"Connection", "keep-alive",
	)
	req.Header.Add(
		"Keep-Alive", fmt.Sprintf("timeout=60, max=%d", m.maxConcurrentRequests),
	)

	rsp, err := m.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to query block height %d from %s: %w",
			height,
			m.maspIndexerApiV1,
			err,
		)
	}
	defer rsp.Body.Close()

	var response serverResponse

	err = json.NewDecoder(bufio.NewReader(rsp.Body)).Decode(&response)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to decode transactions from block height %d: %w",
			height,
			err,
		)
	}

	response.sortMaspTxs()

	return response.Txs, nil
}

func (r *serverResponse) sortMaspTxs() {
	slices.SortFunc(r.Txs, func(t1, t2 Transaction) int {
		return t1.maxMaspTxIndex() - t2.maxMaspTxIndex()
	})
}

func (t *Transaction) maxMaspTxIndex() (max int) {
	if len(t.Batch) == 0 {
		panic("got empty masp transaction batch")
	}
	for i := 0; i < len(t.Batch); i++ {
		if t.Batch[i].MaspTxIndex > max {
			max = t.Batch[i].MaspTxIndex
		}
	}
	return
}
