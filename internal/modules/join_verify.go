package modules

import (
	"fmt"
	"sync"
	"time"

	"github.com/Laky-64/gologging"
	tg "github.com/amarnathcjd/gogram/telegram"

	"main/internal/config"
	"main/internal/core"
	"main/internal/database"
	"main/internal/locales"
	"main/internal/utils"
)

// Join-verify: when someone joins (or asks to join) any chat the bot is in
// (or only the chats listed in VERIFY_CHAT_IDS, if set), the bot sends them a welcome DM with an "Activate" button.
// Pressing it opens the normal /start menu in their DM, so they become a bot
// user. The bot NEVER approves or declines join requests here.
//
// Telegram only lets a bot DM someone who (a) has a pending join request to
// the chat, or (b) already started the bot. For anyone else the DM fails, so
// (optionally, VERIFY_GROUP_FALLBACK) a short welcome message with a
// "Activate" link button is posted in the chat instead.

const (
	verifyCooldown    = 24 * time.Hour
	verifySendGap     = 1100 * time.Millisecond
	verifyGroupMsgTTL = 3 * time.Minute
	verifyCallbackKey = "verify_ok"
)

var verifyState = struct {
	mu      sync.Mutex
	last    map[int64]time.Time
	queue   chan func()
	started sync.Once
}{
	last:  map[int64]time.Time{},
	queue: make(chan func(), 300),
}

// isVerifyChat: if VERIFY_CHAT_IDS is empty, verify runs in EVERY chat the bot
// is in; if it is set, only in those chats.
func isVerifyChat(chatID int64) bool {
	if len(config.VerifyChatIDs) == 0 {
		return true
	}
	for _, id := range config.VerifyChatIDs {
		if id == chatID {
			return true
		}
	}
	return false
}

// verifyShouldNotify allows one welcome per user per 24h (a join request that
// is later approved triggers both events - only the first one goes out).
func verifyShouldNotify(userID int64) bool {
	verifyState.mu.Lock()
	defer verifyState.mu.Unlock()
	if t, ok := verifyState.last[userID]; ok && time.Since(t) < verifyCooldown {
		return false
	}
	if len(verifyState.last) > 5000 {
		for k, t := range verifyState.last {
			if time.Since(t) >= verifyCooldown {
				delete(verifyState.last, k)
			}
		}
	}
	verifyState.last[userID] = time.Now()
	return true
}

// verifyEnqueue runs fn on one background worker, ~1 message/second, so a
// burst of joins can never flood Telegram.
func verifyEnqueue(fn func()) {
	verifyState.started.Do(func() {
		go func() {
			for f := range verifyState.queue {
				func() {
					defer func() {
						if r := recover(); r != nil {
							gologging.ErrorF("join verify: recovered: %v", r)
						}
					}()
					f()
				}()
				time.Sleep(verifySendGap)
			}
		}()
	})
	select {
	case verifyState.queue <- fn:
	default:
		gologging.Warn("join verify: queue full, dropping a welcome message")
	}
}

// verifyTitles remembers group names (filled whenever the bot sees a message
// in a supergroup) so the welcome text can say which group it is about.
var verifyTitles sync.Map // int64 -> string

func rememberChatTitle(chatID int64, title string) {
	if chatID != 0 && title != "" {
		verifyTitles.Store(chatID, title)
	}
}

func verifyChatTitle(chatID int64) string {
	if v, ok := verifyTitles.Load(chatID); ok {
		if t, _ := v.(string); t != "" {
			return utils.EscapeHTML(utils.ShortTitle(t, 40))
		}
	}
	return "our group"
}

func verifyBotName(client *tg.Client) string {
	if me := client.Me(); me != nil && me.FirstName != "" {
		return utils.EscapeHTML(me.FirstName)
	}
	return "Music Bot"
}

func verifyMention(userID int64, u *tg.UserObj) string {
	if u != nil {
		return utils.MentionHTML(u)
	}
	return fmt.Sprintf(`<a href="tg://user?id=%d">there</a>`, userID)
}

func verifyDMText(client *tg.Client, chatID, userID int64, u *tg.UserObj, requested bool) string {
	line := fmt.Sprintf("You're now part of <b>%s</b>.", verifyChatTitle(chatID))
	if requested {
		line = fmt.Sprintf("Your request to join <b>%s</b> is received.", verifyChatTitle(chatID))
	}
	return fmt.Sprintf(`✦ <b>Welcome, %s</b>
%s

<blockquote><b>%s</b>
Studio-grade audio and HD video in your group voice chat. Queue, loop and speed control, all in one tap.</blockquote>

Activate the bot below to unlock every feature.`,
		verifyMention(userID, u), line, verifyBotName(client))
}

func verifyDMMarkup() tg.ReplyMarkup {
	return tg.NewKeyboard().
		AddRow(tg.Button.Data("▶ Activate Music Bot", verifyCallbackKey)).
		Build()
}

// sendVerifyDM returns nil only if the DM really went out.
func sendVerifyDM(client *tg.Client, chatID, userID int64, u *tg.UserObj, requested bool) error {
	_, err := client.SendMessage(userID, verifyDMText(client, chatID, userID, u, requested), &tg.SendOptions{
		ReplyMarkup: verifyDMMarkup(),
		LinkPreview: false,
	})
	return err
}

func sendVerifyGroupFallback(client *tg.Client, chatID, userID int64, u *tg.UserObj) {
	text := fmt.Sprintf("✦ <b>Welcome, %s</b>\n\n<blockquote>Activate %s to play songs and videos in this group.</blockquote>",
		verifyMention(userID, u), verifyBotName(client))
	markup := tg.NewKeyboard().
		AddRow(tg.Button.URL("▶ Activate Music Bot", "https://t.me/"+client.Me().Username+"?start=activate")).
		Build()
	msg, err := client.SendMessage(chatID, text, &tg.SendOptions{ReplyMarkup: markup, LinkPreview: false})
	if err != nil {
		gologging.WarnF("join verify: group message failed in %d: %v", chatID, err)
		return
	}
	// Keep the group clean: remove it after a few minutes (best effort).
	time.AfterFunc(verifyGroupMsgTTL, func() {
		defer func() { _ = recover() }()
		_, _ = msg.Delete()
	})
}

// handleVerifyJoin is called when someone joins a chat (participant update).
func handleVerifyJoin(client *tg.Client, chatID, userID int64, u *tg.UserObj) {
	if !isVerifyChat(chatID) || userID == 0 {
		return
	}
	if u != nil && u.Bot {
		return
	}
	if userID == client.Me().ID || !verifyShouldNotify(userID) {
		return
	}
	verifyEnqueue(func() {
		if err := sendVerifyDM(client, chatID, userID, u, false); err == nil {
			return
		} else {
			gologging.DebugF("join verify: DM to %d failed (%v)", userID, err)
		}
		if config.VerifyGroupFallback {
			sendVerifyGroupFallback(client, chatID, userID, u)
		}
	})
}

// joinRequestHandler: someone asked to join. Only DM them - the request is
// left exactly as it is (not approved, not declined).
func joinRequestHandler(u tg.Update, client *tg.Client) error {
	upd, ok := u.(*tg.UpdateBotChatInviteRequester)
	if !ok || upd == nil {
		return nil
	}
	pc, ok := upd.Peer.(*tg.PeerChannel)
	if !ok || pc == nil {
		return nil
	}
	chatID := int64(-1000000000000) - pc.ChannelID
	if !isVerifyChat(chatID) || upd.UserID == 0 {
		return nil
	}
	if !verifyShouldNotify(upd.UserID) {
		return nil
	}
	userID := upd.UserID
	verifyEnqueue(func() { sendRequesterDM(client, chatID, userID, 0) })
	return nil
}

// Telegram lets the bot DM a join requester right away, but the bot's local
// cache may not know that user yet ("no peer with id ... or missing from
// cache"). The cache fills in a moment later, so retry a few times with
// growing gaps (still well inside Telegram's window for requesters).
var requesterRetryDelays = []time.Duration{
	1 * time.Second, 3 * time.Second, 8 * time.Second, 20 * time.Second,
}

func sendRequesterDM(client *tg.Client, chatID, userID int64, attempt int) {
	err := sendVerifyDM(client, chatID, userID, nil, true)
	if err == nil {
		if attempt > 0 {
			gologging.InfoF("join verify: DM to requester %d sent on retry %d", userID, attempt)
		}
		return
	}
	if attempt >= len(requesterRetryDelays) {
		gologging.WarnF("join verify: DM to requester %d failed after %d retries: %v", userID, attempt, err)
		return
	}
	gologging.DebugF("join verify: DM to requester %d failed (%v), retry %d", userID, err, attempt+1)
	time.AfterFunc(requesterRetryDelays[attempt], func() {
		verifyEnqueue(func() { sendRequesterDM(client, chatID, userID, attempt+1) })
	})
}

// verifyCB: user pressed "Activate" in the DM -> turn the message into
// the normal start menu and register them as a bot user.
func verifyCB(cb *tg.CallbackQuery) error {
	cb.Answer("✅ Bot activated!")
	database.AddServedUser(cb.SenderID)

	caption := F(cb.ChannelID(), "start_private", locales.Arg{
		"user":  utils.MentionHTML(cb.Sender),
		"bot":   utils.MentionHTML(cb.Client.Me()),
		"owner": ownerMention(cb.Client),
	})
	cb.Edit(caption, &tg.SendOptions{ReplyMarkup: core.GetStartMarkup(cb.ChannelID())})

	if config.LoggerID != 0 && isLoggerEnabled() && cb.SenderID != config.OwnerID {
		uName := "N/A"
		if cb.Sender != nil && cb.Sender.Username != "" {
			uName = "@" + cb.Sender.Username
		}
		msg := F(config.LoggerID, "logger_bot_started", locales.Arg{
			"mention":       utils.MentionHTML(cb.Sender),
			"user_id":       cb.SenderID,
			"user_username": uName,
		})
		if _, err := cb.Client.SendMessage(config.LoggerID, msg); err != nil {
			gologging.WarnF("[verify] logger send failed: %v", err)
		}
	}
	return tg.ErrEndGroup
}
