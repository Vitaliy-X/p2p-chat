package chat

type MessageDeduper struct {
	limit int
	seen  map[string]struct{}
	order []string
}

func NewMessageDeduper(limit int) *MessageDeduper {
	if limit <= 0 {
		limit = 1024
	}
	return &MessageDeduper{
		limit: limit,
		seen:  make(map[string]struct{}, limit),
		order: make([]string, 0, limit),
	}
}

func (d *MessageDeduper) SeenOrAdd(id string) bool {
	if _, ok := d.seen[id]; ok {
		return true
	}
	d.seen[id] = struct{}{}
	d.order = append(d.order, id)
	if len(d.order) > d.limit {
		oldest := d.order[0]
		delete(d.seen, oldest)
		copy(d.order, d.order[1:])
		d.order = d.order[:len(d.order)-1]
	}
	return false
}
