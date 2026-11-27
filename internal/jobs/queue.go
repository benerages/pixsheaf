package jobs

import "slices"

// fairQueue keeps one FIFO per client and hands out files round-robin
// across clients (ADR 0022): a client with many files and a client with
// one file alternate.
type fairQueue struct {
	byClient map[string][]*file
	// order lists the clients with queued files; next is where the next
	// pop starts.
	order []string
	next  int
	n     int
}

func newFairQueue() *fairQueue {
	return &fairQueue{byClient: map[string][]*file{}}
}

func (q *fairQueue) len() int { return q.n }

func (q *fairQueue) push(f *file) {
	c := f.job.client
	if len(q.byClient[c]) == 0 {
		q.order = append(q.order, c)
	}
	q.byClient[c] = append(q.byClient[c], f)
	q.n++
}

// pop returns the next file, or nil.
func (q *fairQueue) pop() *file {
	if len(q.order) == 0 {
		return nil
	}
	if q.next >= len(q.order) {
		q.next = 0
	}
	c := q.order[q.next]
	files := q.byClient[c]
	f := files[0]
	files[0] = nil
	files = files[1:]
	q.n--
	if len(files) == 0 {
		delete(q.byClient, c)
		q.order = slices.Delete(q.order, q.next, q.next+1)
		// q.next now points at the following client.
	} else {
		q.byClient[c] = files
		q.next++
	}
	return f
}

// remove drops all queued files for which drop returns true.
func (q *fairQueue) remove(drop func(*file) bool) {
	for i := 0; i < len(q.order); {
		c := q.order[i]
		before := len(q.byClient[c])
		files := slices.DeleteFunc(q.byClient[c], drop)
		q.n -= before - len(files)
		if len(files) > 0 {
			q.byClient[c] = files
			i++
			continue
		}
		delete(q.byClient, c)
		q.order = slices.Delete(q.order, i, i+1)
		if q.next > i {
			q.next--
		}
	}
}
