package utils

import "testing"

func TestFilter(t *testing.T) {
	hits := Filter([]string{}, func(t *string) bool { return len(*t) > 2 })
	if len(hits) != 0 {
		t.Errorf("len(hits) expect 0 but got %d", len(hits))
	}

	hits = Filter([]string{"a", "a", "b", "abc", "def"}, func(t *string) bool { return len(*t) > 2 })
	if len(hits) != 2 {
		t.Errorf("len(hits) expect 2 but got %d", len(hits))
	}
}

func TestFind(t *testing.T) {
	var (
		hit   *int
		exist bool
	)
	_, exist = Find([]int{}, func(i *int) bool { return *i > 0 })
	if exist {
		t.Errorf("exist expect false but got %t", exist)
	}

	hit, _ = Find([]int{-1, 0, 1}, func(i *int) bool { return *i > 0 })
	if *hit != 1 {
		t.Errorf("hit expect 1 but got %d", hit)
	}
}

func TestAny(t *testing.T) {
	exist := Any([]float32{}, func(i *float32) bool { return *i > 0 })
	if exist {
		t.Errorf("exist expect false but got %t", exist)
	}

	exist = Any([]float32{-0.1, 0.0, 0.1}, func(i *float32) bool { return *i > 0 })
	if !exist {
		t.Errorf("exist expect true but got %t", exist)
	}
}
