package pack

import "testing"

func TestPackShelfExactFit(t *testing.T) {
	items := []Item{{ID: "a", Qty: 4, W: 10, D: 10, H: 5, AllowRotate: true}}
	res := PackShelf(items, 20, 20, 10, 0)
	if len(res.Missing) != 0 {
		t.Fatalf("expected no missing, got %+v", res.Missing)
	}
	if res.UsedW != 20 || res.UsedD != 20 {
		t.Fatalf("expected 20x20 used, got %vx%v", res.UsedW, res.UsedD)
	}
	count := 0
	for _, r := range res.Rows {
		count += len(r.Items)
	}
	if count != 4 {
		t.Fatalf("expected 4 placed items, got %d", count)
	}
}

func TestPackShelfTooTall(t *testing.T) {
	items := []Item{{ID: "tall", Qty: 1, W: 5, D: 5, H: 100, AllowRotate: true}}
	res := PackShelf(items, 20, 20, 10, 0)
	if len(res.Missing) != 1 || res.Missing[0].Reason != "too-tall" {
		t.Fatalf("expected too-tall missing, got %+v", res.Missing)
	}
	if res.Missing[0].Requested != 1 || res.Missing[0].Placed != 0 || res.Missing[0].Rejected != 1 {
		t.Fatalf("unexpected missing counts: %+v", res.Missing[0])
	}
}

func TestPackShelfTooWideEvenRotated(t *testing.T) {
	items := []Item{{ID: "wide", Qty: 1, W: 50, D: 50, H: 5, AllowRotate: true}}
	res := PackShelf(items, 20, 100, 10, 0)
	if len(res.Missing) != 1 || res.Missing[0].Reason != "too-wide" {
		t.Fatalf("expected too-wide missing, got %+v", res.Missing)
	}
}

func TestPackShelfNoSpaceOverflow(t *testing.T) {
	// Each item is 10x10; box is 10 wide (1 per row) and 25 deep (2 rows fit, 3rd doesn't).
	items := []Item{{ID: "a", Qty: 3, W: 10, D: 10, H: 5, AllowRotate: false}}
	res := PackShelf(items, 10, 25, 10, 0)
	if len(res.Missing) != 1 {
		t.Fatalf("expected 1 missing entry, got %+v", res.Missing)
	}
	mi := res.Missing[0]
	if mi.Reason != "no-space" || mi.Requested != 3 || mi.Placed != 2 || mi.Rejected != 1 {
		t.Fatalf("unexpected missing: %+v", mi)
	}
}

func TestPackShelfDeterministic(t *testing.T) {
	items := []Item{
		{ID: "b", Qty: 2, W: 5, D: 8, H: 3, AllowRotate: true},
		{ID: "a", Qty: 2, W: 5, D: 8, H: 3, AllowRotate: true},
	}
	r1 := PackShelf(items, 20, 20, 10, 0)
	r2 := PackShelf(items, 20, 20, 10, 0)
	if len(r1.Rows) != len(r2.Rows) {
		t.Fatalf("nondeterministic row count: %d vs %d", len(r1.Rows), len(r2.Rows))
	}
	for i := range r1.Rows {
		if len(r1.Rows[i].Items) != len(r2.Rows[i].Items) {
			t.Fatalf("nondeterministic row %d item count", i)
		}
		for j := range r1.Rows[i].Items {
			if r1.Rows[i].Items[j] != r2.Rows[i].Items[j] {
				t.Fatalf("nondeterministic placement at row %d item %d", i, j)
			}
		}
	}
}

func TestPackShelfPadding(t *testing.T) {
	items := []Item{{ID: "a", Qty: 1, W: 10, D: 10, H: 5, AllowRotate: false}}
	res := PackShelf(items, 20, 20, 10, 2)
	if res.UsedW != 14 || res.UsedD != 14 {
		t.Fatalf("expected 14x14 used with padding 2, got %vx%v", res.UsedW, res.UsedD)
	}
}

func TestPackShelfTooDeepDoesNotBlockSmallItems(t *testing.T) {
	items := []Item{
		{ID: "deep", Qty: 1, W: 5, D: 50, H: 5, AllowRotate: false},
		{ID: "small", Qty: 2, W: 5, D: 5, H: 5, AllowRotate: false},
	}
	res := PackShelf(items, 20, 20, 10, 0)
	if len(res.Missing) != 1 || res.Missing[0].ComponentID != "deep" || res.Missing[0].Reason != "too-deep" {
		t.Fatalf("expected only deep to be too-deep, got %+v", res.Missing)
	}
	count := 0
	for _, r := range res.Rows {
		count += len(r.Items)
	}
	if count != 2 {
		t.Fatalf("expected 2 small items placed, got %d", count)
	}
}

func TestPackShelfNoSpaceDoesNotStopSmallerItems(t *testing.T) {
	// Box is 10 wide, 20 deep. The first 12-deep item fills row 1, leaving 8
	// depth: the second 12-deep item is rejected but the 8-deep one still fits.
	items := []Item{
		{ID: "big", Qty: 2, W: 10, D: 12, H: 5},
		{ID: "small", Qty: 1, W: 10, D: 8, H: 5},
	}
	res := PackShelf(items, 10, 20, 10, 0)
	placed := map[string]int{}
	for _, r := range res.Rows {
		for _, it := range r.Items {
			placed[it.ID]++
		}
	}
	if placed["big"] != 1 || placed["small"] != 1 {
		t.Fatalf("unexpected placement: %+v", placed)
	}
	if len(res.Missing) != 1 {
		t.Fatalf("expected 1 missing entry, got %+v", res.Missing)
	}
	mi := res.Missing[0]
	if mi.ComponentID != "big" || mi.Reason != "no-space" || mi.Requested != 2 || mi.Placed != 1 || mi.Rejected != 1 {
		t.Fatalf("unexpected missing: %+v", mi)
	}
}
