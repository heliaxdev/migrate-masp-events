package main

import (
	"bufio"
	"fmt"
	"log"
	"net/http"
	"slices"
	"strings"

	"github.com/segmentio/encoding/json"
)

const MaxConcurrentRequests = 100

type maspIndexerHealthResponse struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
}

type MaspIndexerClient struct {
	// URL pointing to https://.../api/v1
	maspIndexerApiV1 string
	// Custom client config
	client http.Client
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

func NewMaspIndexerClient(url string) (*MaspIndexerClient, error) {
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

	if health.Version != "1.2.1" {
		return fmt.Errorf("using invalid version %s, expected 1.2.1", health.Version)
	}
	if health.Commit != "6d6d022588a56f7c836e485de257e93646cd7847" {
		if !invalidCommitNotErr {
			return fmt.Errorf(
				"using invalid commit %q, expected 6d6d022588a56f7c836e485de257e93646cd7847",
				health.Commit,
			)
		}
		if health.Commit == "" {
			health.Commit = "<unknown-commit>"
		}
		log.Println(
			"warning: masp indexer commit does not match release 1.2.1,",
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

// TODO: if we care enough, use batch transfers to reduce the nr of requests
// to the masp indexer webserver
func (m *MaspIndexerClient) BlockHeight(height int) ([]Transaction, error) {
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
		"Keep-Alive", fmt.Sprintf("timeout=60, max=%d", MaxConcurrentRequests),
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
