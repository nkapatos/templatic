package main

import (
	"cmp"
	"fmt"
	"math/rand/v2"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"examples/table/components"
)

var (
	customers = []string{"Acme", "Globex", "Initech", "Umbrella", "Hooli", "Stark", "Wayne", "Wonka", "Tyrell", "Cyberdyne", "Soylent", "Vandelay"}
	statuses  = []string{"pending", "paid", "shipped", "refunded"}
)

// store holds the orders; it is safe for concurrent use.
type store struct {
	mu    sync.Mutex
	rng   *rand.Rand
	rows  []components.Order
	fresh time.Time // the time of the latest tick
}

// newStore returns n generated orders, the same for every run.
func newStore(n int, now time.Time) *store {
	st := &store{rng: rand.New(rand.NewPCG(6, 6))}
	for range n {
		st.add(now.Add(-time.Duration(st.rng.IntN(72*3600)) * time.Second))
	}
	return st
}

func (st *store) add(at time.Time) {
	st.rows = append(st.rows, components.Order{
		ID:       1000 + len(st.rows),
		Customer: fmt.Sprintf("%s %d", customers[st.rng.IntN(len(customers))], 1+st.rng.IntN(9)),
		Status:   statuses[st.rng.IntN(len(statuses))],
		Amount:   500 + st.rng.IntN(250000),
		Updated:  at,
	})
}

// tick changes a few orders and sometimes adds one, all stamped now.
func (st *store) tick(now time.Time) {
	st.mu.Lock()
	defer st.mu.Unlock()
	now = now.Truncate(time.Second)
	for range 1 + st.rng.IntN(3) {
		o := &st.rows[st.rng.IntN(len(st.rows))]
		o.Status = statuses[st.rng.IntN(len(statuses))]
		o.Amount = max(100, o.Amount+st.rng.IntN(5000)-2500)
		o.Updated = now
	}
	if st.rng.IntN(2) == 0 {
		st.add(now)
	}
	st.fresh = now
}

// Len returns how many orders there are.
func (st *store) Len() int {
	st.mu.Lock()
	defer st.mu.Unlock()
	return len(st.rows)
}

// Query returns the page of orders v selects, with v's page clamped to the
// pages there are.
func (st *store) Query(v components.View) components.Result {
	st.mu.Lock()
	var rows []components.Order
	q := strings.ToLower(v.Q)
	for _, o := range st.rows {
		if q == "" || strings.Contains(strings.ToLower(o.Customer), q) || strings.Contains(strconv.Itoa(o.ID), q) {
			rows = append(rows, o)
		}
	}
	fresh := st.fresh
	st.mu.Unlock()

	slices.SortStableFunc(rows, func(a, b components.Order) int {
		c := compare(v.Sort, a, b)
		if c == 0 {
			c = cmp.Compare(a.ID, b.ID)
		}
		if v.Dir == "desc" {
			return -c
		}
		return c
	})
	pages := max(1, (len(rows)+components.PageSize-1)/components.PageSize)
	v.Page = min(v.Page, pages)
	lo := (v.Page - 1) * components.PageSize
	hi := min(lo+components.PageSize, len(rows))
	return components.Result{View: v, Rows: rows[lo:hi], Total: len(rows), Pages: pages, Fresh: fresh}
}

func compare(col string, a, b components.Order) int {
	switch col {
	case "id":
		return cmp.Compare(a.ID, b.ID)
	case "customer":
		return strings.Compare(a.Customer, b.Customer)
	case "status":
		return strings.Compare(a.Status, b.Status)
	case "amount":
		return cmp.Compare(a.Amount, b.Amount)
	default:
		return a.Updated.Compare(b.Updated)
	}
}
