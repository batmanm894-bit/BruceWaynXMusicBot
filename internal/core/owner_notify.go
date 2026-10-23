package core

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Laky-64/gologging"
	"github.com/amarnathcjd/gogram/telegram"

	"main/internal/config"
)

// Errors/alerts go ONLY to the owner's DM (never the log group), and at
// most ONE message per hour. Anything that happens inside that hour is
// collected and sent together as a single digest (same error repeated =
// shown once with a count).
const (
	ownerNotifyInterval = time.Hour
	ownerNotifyMaxMsg   = 3500
	ownerNotifyMaxKeys  = 30
)

type ownerNotice struct {
	text  string
	count int
}

var ownerNotifier = struct {
	mu       sync.Mutex
	pending  map[string]*ownerNotice
	order    []string
	dropped  int
	lastSent time.Time
	timerSet bool
}{pending: map[string]*ownerNotice{}}

// NotifyOwner queues an HTML-formatted notice for the owner's DM.
// key identifies "the same error" so repeats are merged into one line
// with a counter. Never blocks the caller.
func NotifyOwner(key, htmlText string) {
	if config.OwnerID == 0 {
		return
	}
	n := &ownerNotifier
	n.mu.Lock()
	defer n.mu.Unlock()

	if p, ok := n.pending[key]; ok {
		p.count++
	} else if len(n.order) >= ownerNotifyMaxKeys {
		n.dropped++
	} else {
		n.pending[key] = &ownerNotice{text: htmlText, count: 1}
		n.order = append(n.order, key)
	}

	if n.timerSet {
		return
	}
	wait := time.Until(n.lastSent.Add(ownerNotifyInterval))
	if wait < 0 {
		wait = 0
	}
	n.timerSet = true
	time.AfterFunc(wait, flushOwnerNotices)
}

func flushOwnerNotices() {
	defer func() {
		if r := recover(); r != nil {
			gologging.ErrorF("owner notify: recovered: %v", r)
		}
	}()

	n := &ownerNotifier
	n.mu.Lock()
	items := make([]*ownerNotice, 0, len(n.order))
	for _, k := range n.order {
		items = append(items, n.pending[k])
	}
	dropped := n.dropped
	n.pending = map[string]*ownerNotice{}
	n.order = nil
	n.dropped = 0
	n.timerSet = false
	n.lastSent = time.Now()
	n.mu.Unlock()

	if len(items) == 0 || Bot == nil || config.OwnerID == 0 {
		return
	}

	var sb strings.Builder
	skipped := 0
	for i, it := range items {
		part := it.text
		if it.count > 1 {
			part += fmt.Sprintf("\n<b>x%d</b> (is 1 ghante mein)", it.count)
		}
		if i > 0 && sb.Len()+len(part) > ownerNotifyMaxMsg {
			skipped = len(items) - i
			break
		}
		if i > 0 {
			sb.WriteString("\n\n-----\n\n")
		}
		sb.WriteString(part)
	}
	if extra := skipped + dropped; extra > 0 {
		sb.WriteString(fmt.Sprintf("\n\n+%d aur alag errors (logs dekho)", extra))
	}

	if _, err := Bot.SendMessage(
		config.OwnerID,
		sb.String(),
		&telegram.SendOptions{ParseMode: "HTML"},
	); err != nil {
		gologging.WarnF("owner notify: failed to DM owner: %v", err)
	}
}
