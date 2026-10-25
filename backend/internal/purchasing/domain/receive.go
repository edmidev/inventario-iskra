package domain

import (
	"fmt"
	"strings"
	"time"
)

// ReceiveLine asks to receive part of one order line.
type ReceiveLine struct {
	LineID   int64
	Quantity int
}

// ReceiveRequest registers a delivery.
type ReceiveRequest struct {
	LocationID int64
	Note       string
	// ClientKey makes the request idempotent: repeating it returns the
	// original receipt without side effects.
	ClientKey string
	Lines     []ReceiveLine
}

// PlanReceipt validates a receive request against the order and returns it
// normalized plus, for each requested line, its index in o.Lines. It never
// mutates the order. Only ordered and partially received orders accept goods.
func (o Order) PlanReceipt(req ReceiveRequest) (ReceiveRequest, []int, error) {
	if o.Status != StatusOrdered && o.Status != StatusPartiallyReceived {
		return ReceiveRequest{}, nil, &InvalidStateError{Status: o.Status, Action: "receive"}
	}
	req.Note = strings.TrimSpace(req.Note)
	req.ClientKey = strings.TrimSpace(req.ClientKey)
	fe := map[string]string{}
	if req.LocationID <= 0 {
		fe["location_id"] = "required"
	}
	if len(req.Note) > MaxReceiptNote {
		fe["note"] = "up to 400 characters"
	}
	if len(req.ClientKey) > MaxClientKeyLen {
		fe["client_key"] = "up to 100 characters"
	}
	if len(req.Lines) == 0 {
		fe["lines"] = "at least one line is required"
	}
	idx := make([]int, len(req.Lines))
	seen := map[int64]bool{}
	over := map[string]string{}
	for i, rl := range req.Lines {
		p := fmt.Sprintf("lines[%d].", i)
		at := -1
		for j, l := range o.Lines {
			if l.ID == rl.LineID {
				at = j
				break
			}
		}
		idx[i] = at
		switch {
		case at < 0:
			fe[p+"line_id"] = "unknown line"
			continue
		case seen[rl.LineID]:
			fe[p+"line_id"] = "repeated line"
			continue
		}
		seen[rl.LineID] = true
		switch rem := o.Lines[at].Remaining(); {
		case rl.Quantity <= 0:
			fe[p+"quantity"] = "must be greater than 0"
		case rl.Quantity > rem:
			over[p+"quantity"] = CodeOverReceipt
		}
	}
	if len(fe) > 0 || len(over) > 0 {
		all := map[string]string{}
		for k, v := range over {
			all[k] = v
		}
		for k, v := range fe {
			all[k] = v
		}
		ve := &ValidationError{Fields: all}
		if len(fe) == 0 {
			ve.Code = CodeOverReceipt
			ve.Message = "received quantity exceeds what is still pending on the line"
		}
		return ReceiveRequest{}, nil, ve
	}
	return req, idx, nil
}

// ApplyReceipt records a receipt: it bumps the received quantities, appends
// the receipt and moves the order to partially_received or received.
func (o *Order) ApplyReceipt(rc Receipt, now time.Time) {
	for _, rl := range rc.Lines {
		for j := range o.Lines {
			if o.Lines[j].ID == rl.LineID {
				o.Lines[j].ReceivedQty += rl.Quantity
			}
		}
	}
	o.Receipts = append(o.Receipts, rc)
	o.Status = StatusReceived
	for _, l := range o.Lines {
		if l.Remaining() > 0 {
			o.Status = StatusPartiallyReceived
			break
		}
	}
	o.UpdatedAt = now
}
