package main

import (
	"context"
	"encoding/json"
	"os/exec"
	"time"
)

type Usage struct {
	V         int     `json:"v"`
	T         string  `json:"t"`
	Tokens    int64   `json:"tokens"`
	Cost      float64 `json:"cost"`
	Active    bool    `json:"active"`
	BlockLeft int64   `json:"block_left"` // detik tersisa di blok 5 jam
}

type Source interface {
	Fetch(ctx context.Context) (Usage, error)
}

type CcusageSource struct{}

func (CcusageSource) Fetch(ctx context.Context) (Usage, error) {
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()

	name, args := "ccusage", []string{"blocks", "--active", "--json"}
	if _, err := exec.LookPath("ccusage"); err != nil {
		name, args = "npx", []string{"--yes", "ccusage", "blocks", "--active", "--json"}
	}
	cmd := exec.CommandContext(ctx, name, args...)
	hideWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		return Usage{}, err
	}

	var r struct {
		Blocks []struct {
			TotalTokens int64   `json:"totalTokens"`
			CostUSD     float64 `json:"costUSD"`
			EndTime     string  `json:"endTime"`
		} `json:"blocks"`
	}
	if err := json.Unmarshal(out, &r); err != nil {
		return Usage{}, err
	}

	u := Usage{V: 1, T: "usage"}
	if len(r.Blocks) == 0 {
		return u, nil // tidak ada blok aktif = idle
	}
	b := r.Blocks[0]
	u.Tokens, u.Cost, u.Active = b.TotalTokens, b.CostUSD, true
	if end, err := time.Parse(time.RFC3339, b.EndTime); err == nil {
		if left := int64(time.Until(end).Seconds()); left > 0 {
			u.BlockLeft = left
		}
	}
	return u, nil
}
