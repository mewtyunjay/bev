package main

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"
)

func main() {
	os.Exit(run())
}

func run() int {
	if len(os.Args) > 1 && os.Args[1] == "--help" {
		fmt.Println("Usage: bev classify < input")
		fmt.Println("Classifies stdin as shell, codex, or hold using Jev.")
		fmt.Println("Environment: TYPESAFE_API_KEY (required), BEV_MODEL (default jev-1.13.0), BEV_MIN_PROBABILITY (default 0.90), BEV_TIMEOUT (default 2s)")
		return 0
	}
	if len(os.Args) != 2 || os.Args[1] != "classify" {
		fail("usage: bev classify < input (try --help)")
		return 2
	}
	line, err := readLine(os.Stdin)
	if err != nil {
		fail(err.Error())
		return 1
	}
	if strings.TrimSpace(line) == "" {
		fmt.Println("shell")
		return 0
	}
	key := os.Getenv("TYPESAFE_API_KEY")
	if key == "" {
		fail("TYPESAFE_API_KEY is required")
		return 1
	}
	model := os.Getenv("BEV_MODEL")
	if model == "" {
		model = defaultModel
	}
	min := 0.90
	if v := os.Getenv("BEV_MIN_PROBABILITY"); v != "" {
		min, err = strconv.ParseFloat(v, 64)
		if err != nil || math.IsNaN(min) || math.IsInf(min, 0) || min <= 0.5 || min > 1 {
			fail("BEV_MIN_PROBABILITY must be a finite number in (0.5,1]")
			return 1
		}
	}
	timeoutValue := 2 * time.Second
	if v := os.Getenv("BEV_TIMEOUT"); v != "" {
		timeoutValue, err = timeout(v)
		if err != nil {
			fail(err.Error())
			return 1
		}
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	c := &classifier{client: &http.Client{Timeout: timeoutValue, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, endpoint: defaultEndpoint, model: model, minProb: min}
	result, err := c.classify(ctx, key, line)
	if err != nil {
		fail(err.Error())
		return 1
	}
	fmt.Println(result)
	return 0
}

func fail(msg string) { fmt.Fprintln(os.Stderr, "bev: "+msg) }
