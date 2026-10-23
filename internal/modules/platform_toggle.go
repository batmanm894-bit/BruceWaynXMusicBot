package modules

import (
	"strings"

	"github.com/Laky-64/gologging"
	tg "github.com/amarnathcjd/gogram/telegram"

	"main/internal/platforms"
	"main/internal/utils"
)

func init() {
	helpTexts["/platform"] = `<i>Switch a song source (platform) on or off.</i>

<u>Usage:</u>
<b>/platform</b> - show all platforms (on/off)
<b>/platform shruti off</b> - ShrutiAPI will not be tried at all
<b>/platform shruti on</b> - turn it back on
<b>/platform shruti</b> - toggle
<b>/platform all on</b> - turn everything back on

<b>Names:</b> shruti, fallen, saavn, ytdlp, soundcloud, spotify, telegram, directstream

<b>Notes:</b>
- Saved in the database, stays after restart
- YtDlp off = no yt-dlp downloads (YouTube search keeps working)
- YouTube search itself cannot be turned off
- <b>Owner only</b>`
	helpTexts["/platforms"] = helpTexts["/platform"]
}

func platformStatusText() string {
	var sb strings.Builder
	sb.WriteString("<b>Platforms</b>\n\n")
	for _, n := range platforms.RegisteredNames() {
		if n == platforms.PlatformYouTube {
			continue
		}
		if platforms.IsDisabled(n) {
			sb.WriteString("OFF  <b>" + string(n) + "</b>\n")
		} else {
			sb.WriteString("ON   <b>" + string(n) + "</b>\n")
		}
	}
	sb.WriteString("\n<i>Use:</i> <code>/platform shruti off</code> | <code>/platform shruti on</code> | <code>/platform all on</code>")
	return sb.String()
}

func platformToggleHandler(m *tg.NewMessage) error {
	args := strings.Fields(m.Text())
	if len(args) < 2 {
		m.Reply(platformStatusText())
		return tg.ErrEndGroup
	}

	target := strings.ToLower(args[1])
	if target == "all" || target == "reset" {
		if len(args) >= 3 {
			if on, err := utils.ParseBool(args[2]); err == nil && !on {
				m.Reply("Sab platform ek saath off nahi kar sakte. Ek-ek karke off karo.")
				return tg.ErrEndGroup
			}
		}
		if err := platforms.EnableAll(); err != nil {
			m.Reply("Save nahi hua: " + err.Error())
			return tg.ErrEndGroup
		}
		gologging.Info("All platforms switched on by owner")
		m.Reply("Sab platforms ON kar diye.\n\n" + platformStatusText())
		return tg.ErrEndGroup
	}

	name, ok := platforms.ResolvePlatformName(args[1])
	if !ok {
		m.Reply("Ye platform nahi mila: <code>" + args[1] + "</code>\n\n" + platformStatusText())
		return tg.ErrEndGroup
	}
	if name == platforms.PlatformYouTube {
		m.Reply("YouTube search band nahi ho sakta (bot ki search isi se hoti hai).")
		return tg.ErrEndGroup
	}

	off := !platforms.IsDisabled(name) // default: toggle
	if len(args) >= 3 {
		on, err := utils.ParseBool(args[2])
		if err != nil {
			m.Reply("Last mein <b>on</b> ya <b>off</b> likho.\nExample: <code>/platform shruti off</code>")
			return tg.ErrEndGroup
		}
		off = !on
	}

	if err := platforms.SetDisabled(name, off); err != nil {
		m.Reply("Save nahi hua (restart ke baad wapas on ho sakta hai): " + err.Error())
		return tg.ErrEndGroup
	}

	state := "ON"
	if off {
		state = "OFF"
	}
	gologging.InfoF("Platform %s switched %s by owner", name, state)
	m.Reply("<b>" + string(name) + "</b> ab <b>" + state + "</b> hai.\n\n" + platformStatusText())
	return tg.ErrEndGroup
}
