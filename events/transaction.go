package events

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

var txCounter int64 // replace this with something that won't break

// nextTXID provides the next transaction identifier.
func nexttxID() int64 {
	// TODO(stevvooe): Need to coordinate this with existing transaction logs.
	// For now, this is a toy, but not a racy one.
	return atomic.AddInt64(&txCounter, 1)
}

type transaction struct {
	ctx        context.Context
	id         int64
	parent     *transaction // if nil, no parent transaction
	finish     sync.Once
	start, end time.Time // informational
}

// begin creates a sub-transaction.
func (tx *transaction) begin(ctx context.Context, poster Poster) *transaction {
	id := nexttxID()

	child := &transaction{
		ctx:    ctx,
		id:     id,
		parent: tx,
		start:  time.Now(),
	}

	// post the transaction started event
	poster.Post(ctx, child.makeTransactionEvent("begin")) // tranactions are really just events

	return child
}

// commit writes out the transaction.
func (tx *transaction) commit(poster Poster) {
	tx.finish.Do(func() {
		tx.end = time.Now()
		poster.Post(tx.ctx, tx.makeTransactionEvent("commit"))
	})
}

func (tx *transaction) rollback(poster Poster, cause error) {
	tx.finish.Do(func() {
		tx.end = time.Now()
		event := tx.makeTransactionEvent("rollback")
		event = fmt.Sprintf("%s error=%q", event, cause.Error())
		poster.Post(tx.ctx, event)
	})
}

func (tx *transaction) makeTransactionEvent(status string) Event {
	// TODO(stevvooe): obviously, we need more structure than this.
	event := fmt.Sprintf("%v %v", status, tx.id)
	if tx.parent != nil {
		event += " parent=" + fmt.Sprint(tx.parent.id)
	}

	return event
}

type txKey struct{}

func getTx(ctx context.Context) (*transaction, bool) {
	tx := ctx.Value(txKey{})
	if tx == nil {
		return nil, false
	}

	return tx.(*transaction), true
}

// WithTx returns a new context with an event transaction, such that events
// posted to the underlying context will be committed to the event log as a
// group, organized by a transaction id, when commit is called.
func WithTx(pctx context.Context) (ctx context.Context, commit func(), rollback func(err error)) {
	poster := G(pctx)
	parent, _ := getTx(pctx)
	tx := parent.begin(pctx, poster)

	return context.WithValue(pctx, txKey{}, tx), func() {
			tx.commit(poster)
		}, func(err error) {
			tx.rollback(poster, err)
		}
}
