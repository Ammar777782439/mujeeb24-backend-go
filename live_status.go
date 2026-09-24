package main

import (
	"context"
	"fmt"
	"log"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	dbURL := "postgres://mujeeb:551f7de64c01ce392e5a8ce35d8c8a66447e6d90fe8bd9de@127.0.0.1:5433/mujeeb24?sslmode=disable"
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatalf("pgxpool error: %v", err)
	}
	defer pool.Close()

	fmt.Println("=== 1. Latest 3 Communication Messages ===")
	rows, err := pool.Query(ctx, `SELECT id, direction, origin, text_content, created_at FROM communication_messages ORDER BY created_at DESC LIMIT 3`)
	if err != nil {
		log.Fatal(err)
	}
	for rows.Next() {
		var id, dir, orig, text string
		var createdAt any
		rows.Scan(&id, &dir, &orig, &text, &createdAt)
		fmt.Printf("[%v] Dir: %s | Origin: %s | Text: %s\n", createdAt, dir, orig, text)
	}
	rows.Close()

	fmt.Println("\n=== 2. Latest 3 AI Decisions ===")
	rows2, err := pool.Query(ctx, `SELECT id, intent_base, requested_action, policy_decision, requires_human, reason_codes, created_at FROM ai_decisions ORDER BY created_at DESC LIMIT 3`)
	if err != nil {
		log.Fatal(err)
	}
	for rows2.Next() {
		var id, intent, action string
		var pol *string
		var reqHuman bool
		var reasons []byte
		var createdAt any
		rows2.Scan(&id, &intent, &action, &pol, &reqHuman, &reasons, &createdAt)
		p := "<nil>"
		if pol != nil {
			p = *pol
		}
		fmt.Printf("[%v] ID: %s | Intent: %s | Action: %s | Pol: %s | ReqH: %v | Reasons: %s\n", createdAt, id, intent, action, p, reqHuman, string(reasons))
	}
	rows2.Close()

	fmt.Println("\n=== 3. Latest 3 Outbox Entries ===")
	rows3, err := pool.Query(ctx, `SELECT id, status, destination_type, error_reason, created_at FROM outbox_entries ORDER BY created_at DESC LIMIT 3`)
	if err != nil {
		log.Fatal(err)
	}
	for rows3.Next() {
		var id, status, dest string
		var errReason *string
		var createdAt any
		rows3.Scan(&id, &status, &dest, &errReason, &createdAt)
		er := "<nil>"
		if errReason != nil {
			er = *errReason
		}
		fmt.Printf("[%v] ID: %s | Status: %s | Dest: %s | Err: %s\n", createdAt, id, status, dest, er)
	}
	rows3.Close()

	fmt.Println("\n=== 4. Conversation Details ===")
	rows4, err := pool.Query(ctx, `SELECT id, state, ownership, ai_mode_override, last_activity_at FROM conversations ORDER BY last_activity_at DESC LIMIT 1`)
	if err != nil {
		log.Fatal(err)
	}
	for rows4.Next() {
		var id, state, own string
		var aiMode *string
		var lastAct any
		rows4.Scan(&id, &state, &own, &aiMode, &lastAct)
		m := "<nil>"
		if aiMode != nil {
			m = *aiMode
		}
		fmt.Printf("[%v] Conv: %s | State: %s | Own: %s | AIMode: %s\n", lastAct, id, state, own, m)
	}
	rows4.Close()
}
