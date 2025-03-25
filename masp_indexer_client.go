package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
)

const MaxConcurrentRequests = 100

type MaspIndexerClient struct {
	// URL pointing to https://.../api/v1
	maspIndexerApiV1 string
	// Custom client config
	client *http.Client
}

type TransactionSlot struct {
	MaspTxIndex int `json:"masp_tx_index"`
}

type Transaction struct {
	BlockIndex int               `json:"block_index"`
	Batch      []TransactionSlot `json:"batch"`
}

type serverResponse struct {
	Txs []Transaction `json:"txs"`
}

func NewMaspIndexerClient(url string) *MaspIndexerClient {
	t := http.DefaultTransport.(*http.Transport).Clone()

	// Use either HTTP/1 and HTTP/2.
	t.Protocols = new(http.Protocols)
	t.Protocols.SetHTTP1(true)
	t.Protocols.SetHTTP2(true)

	cli := &http.Client{Transport: t}

	return &MaspIndexerClient{
		maspIndexerApiV1: url,
		client:           cli,
	}
}

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
		"Keep-Alive", fmt.Sprintf("timeout=60, max=%s", MaxConcurrentRequests),
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

	return response.Txs, nil
}
