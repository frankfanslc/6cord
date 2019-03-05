package main

import (
	"flag"
	"log"
	"os"
	"strings"

	"github.com/davecgh/go-spew/spew"
	"github.com/gdamore/tcell"
	"github.com/rivo/tview"
	"github.com/rumblefrog/discordgo"
	keyring "github.com/zalando/go-keyring"
	"gitlab.com/diamondburned/6cord/md"
)

const (
	// AppName used for keyrings
	AppName = "6cord"

	// DefaultStatus is used as the default message in the status bar
	DefaultStatus = "Send a message or input a command"
)

var (
	app           = tview.NewApplication()
	appflex       = tview.NewFlex()
	rightflex     = tview.NewFlex()
	guildView     = tview.NewTreeView()
	messagesView  = tview.NewTextView()
	messagesFrame = tview.NewFrame(messagesView)
	wrapFrame     *tview.Frame
	input         = tview.NewInputField()
	autocomp      = tview.NewList()

	// ChannelID stores the current channel's ID
	ChannelID int64

	// LastAuthor stores for appending messages
	// TODO: migrate to table + lastRow
	LastAuthor int64

	foregroundColor int

	d *discordgo.Session
)

func init() {
	md.HighlightStyle = SyntaxHighlightColorscheme

	app.SetBeforeDrawFunc(func(s tcell.Screen) bool {
		s.Clear()
		return false
	})

	commands = append(commands, CustomCommands...)
}

func main() {
	token := flag.String("t", "", "Discord token (1)")

	username := flag.String("u", "", "Username/Email (2)")
	password := flag.String("p", "", "Password (2)")

	debug := flag.Bool("d", false, "Logs extra events")
	fgColor := flag.Int("fgc", 15, "Default foreground color, 0-255, 0 is black, 15 is white")

	flag.Parse()

	foregroundColor = *fgColor

	tview.Borders.HorizontalFocus = tview.Borders.Horizontal
	tview.Borders.VerticalFocus = tview.Borders.Vertical

	tview.Borders.TopLeftFocus = tview.Borders.TopLeft
	tview.Borders.TopRightFocus = tview.Borders.TopRight
	tview.Borders.BottomLeftFocus = tview.Borders.BottomLeft
	tview.Borders.BottomRightFocus = tview.Borders.BottomRight

	tview.Borders.Horizontal = ' '
	tview.Borders.Vertical = ' '

	tview.Borders.TopLeft = ' '
	tview.Borders.TopRight = ' '
	tview.Borders.BottomLeft = ' '
	tview.Borders.BottomRight = ' '

	guildView.SetInputCapture(func(ev *tcell.EventKey) *tcell.EventKey {
		// workaround to prevent crash when no root in tree
		return nil
	})

	messagesView.SetRegions(true)
	messagesView.SetWrap(true)
	messagesView.SetWordWrap(false)
	messagesView.SetScrollable(true)
	messagesView.SetDynamicColors(true)
	messagesView.SetTextColor(tcell.Color(foregroundColor))
	messagesView.SetBackgroundColor(BackgroundColor)
	messagesView.SetText(`    [::b]Quick Start[::-]
        - Right arrow from guild list to focus to input
		- Left arrow from input to focus to guild list
		- Up arrow from input to go to autocomplete/message scrollback
		- Tab to show/hide channels
		- /goto [#channel] jumps to that channel`)

	var (
		login []interface{}
		err   error
	)

	switch {
	case *token != "":
		login = append(login, *token)

		if err := keyring.Delete(AppName, "token"); err == nil {
			log.Println("Keyring deleted.")
		}

	case *username != "", *password != "":
		login = append(login, *username)
		login = append(login, *password)

		if *token != "" {
			login = append(login, *token)
		}

	default:
		k, err := keyring.Get(AppName, "token")
		if err != nil {
			log.Fatalln("Token OR username + password missing! Refer to -h")
		}

		login = append(login, k)
	}

	d, err = discordgo.New(login...)
	if err != nil {
		panic(err)
	}

	d.UserAgent = `Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/70.0.3534.4 Safari/537.36`

	d.State.MaxMessageCount = 50

	// Main app page

	appflex.SetDirection(tview.FlexColumn)
	appflex.SetBackgroundColor(BackgroundColor)

	{ // Left container
		guildView.SetPrefixes([]string{"", ""})
		guildView.SetTopLevel(1)
		guildView.SetAlign(false)
		guildView.SetBorder(true)
		guildView.SetBorderAttributes(tcell.AttrDim)
		guildView.SetBorderPadding(0, 0, 1, 1)
		guildView.SetTitle("[Servers[]")
		guildView.SetTitleAlign(tview.AlignLeft)

		guildView.SetBackgroundColor(BackgroundColor)
		guildView.SetGraphicsColor(tcell.Color(foregroundColor))
		guildView.SetTitleColor(tcell.Color(foregroundColor))

		guildView.SetInputCapture(func(ev *tcell.EventKey) *tcell.EventKey {
			return nil
		})

		appflex.AddItem(guildView, 0, 1, true)
	}

	{ // Right container
		rightflex.SetDirection(tview.FlexRow)
		rightflex.SetBackgroundColor(BackgroundColor)

		wrapFrame = tview.NewFrame(rightflex)
		wrapFrame.SetBorder(true)
		wrapFrame.SetBorderAttributes(tcell.AttrDim)
		wrapFrame.SetBorders(0, 0, 0, 0, 0, 0)
		wrapFrame.SetTitle("")
		wrapFrame.SetTitleAlign(tview.AlignLeft)
		wrapFrame.SetTitleColor(tcell.Color(foregroundColor))
		wrapFrame.SetBackgroundColor(BackgroundColor)

		autocomp.ShowSecondaryText(false)
		autocomp.SetBackgroundColor(BackgroundColor)
		autocomp.SetMainTextColor(tcell.Color(foregroundColor))
		autocomp.SetSelectedTextColor(tcell.Color(15 - foregroundColor))
		autocomp.SetSelectedBackgroundColor(tcell.Color(foregroundColor))
		autocomp.SetShortcutColor(tcell.Color(foregroundColor))

		autocomp.SetInputCapture(func(ev *tcell.EventKey) *tcell.EventKey {
			switch ev.Key() {
			case tcell.KeyDown:
				if autocomp.GetCurrentItem()+1 == autocomp.GetItemCount() {
					app.SetFocus(input)
					return nil
				}

				return ev

			case tcell.KeyUp:
				if autocomp.GetCurrentItem() == 0 {
					app.SetFocus(input)
					return nil
				}

				return ev

			case tcell.KeyEnter:
				return ev
			}

			if ev.Rune() >= 0x31 && ev.Rune() <= 0x122 {
				return ev
			}

			app.SetFocus(input)
			return nil
		})

		resetInputBehavior()
		input.SetInputCapture(inputKeyHandler)
		input.SetFieldTextColor(tcell.Color(foregroundColor))

		input.SetChangedFunc(func(text string) {
			if len(text) == 0 {
				clearList()
				stateResetter()
				return
			}

			if text == "/" {
				fuzzyCommands(text)
				return
			}

			if string(text[len(text)-1]) == " " {
				clearList()
				stateResetter()
				return
			}

			words := strings.Fields(text)

			if len(words) < 1 {
				clearList()
				stateResetter()
				return
			}

			switch last := words[len(words)-1]; {
			case strings.HasPrefix(last, "@"):
				fuzzyMentions(last)
			case strings.HasPrefix(last, "#"):
				fuzzyChannels(last)
			case strings.HasPrefix(last, ":"):
				fuzzyEmojis(last)
			case strings.HasPrefix(last, "~"):
				fuzzyMessages(last)
			case strings.HasPrefix(text, "/upload "):
				fuzzyUpload(text)
			case strings.HasPrefix(text, "/"):
				if len(words) == 1 {
					fuzzyCommands(text)
				}
			default:
				typingTrigger()
				clearList()
				stateResetter()
			}
		})

		messagesFrame.SetBorders(0, 0, 0, 0, 0, 0)
		messagesFrame.SetBackgroundColor(BackgroundColor)

		rightflex.AddItem(messagesFrame, 0, 1, false)
		rightflex.AddItem(autocomp, 1, 1, true)
		rightflex.AddItem(input, 1, 1, true)
		rightflex.SetBackgroundColor(BackgroundColor)

		appflex.AddItem(wrapFrame, 0, 2, true)
	}

	messagesView.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyPgDn, tcell.KeyPgUp, tcell.KeyUp, tcell.KeyDown:
			handleScroll()
			return event

		case tcell.KeyLeft:
			app.SetFocus(guildView)
			return nil
		}

		switch event.Rune() {
		case 'j', 'k':
			handleScroll()
			return event

		case 'g', 'G':
			return event
		}

		resetInputBehavior()
		app.SetFocus(input)
		return nil
	})

	autocomp.SetSelectedFunc(func(i int, a, b string, c rune) {
		autofillfunc(i)
	})

	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyF5:
			go func() {
				app.Stop()
				app.Run()
			}()

		case tcell.KeyTab:
			toggleChannels()
			app.ForceDraw()
		}

		return event
	})

	app.SetRoot(appflex, true)

	toggleChannels()

	logFile, err := os.OpenFile(
		os.TempDir()+"/6cord.log",
		os.O_RDWR|os.O_CREATE|os.O_APPEND|os.O_SYNC,
		0664,
	)

	if err != nil {
		panic(err)
	}

	defer logFile.Close()

	log.SetOutput(logFile)
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	discordgo.Logger = func(msgL, caller int, format string, a ...interface{}) {
		log.Println("Discordgo:", msgL, caller, format, a)

		if *debug {
			// Unsure if I should have spew as a dependency
			log.Println(spew.Sdump(a))
		}
	}

	d.AddHandler(onReady)
	d.AddHandler(messageCreate)
	d.AddHandler(messageUpdate)
	d.AddHandler(messageDelete)
	d.AddHandler(messageDeleteBulk)
	d.AddHandler(reactionAdd)
	d.AddHandler(reactionRemove)
	d.AddHandler(reactionRemoveAll)
	// d.AddHandler(onTyping) - still broken
	d.AddHandler(messageAck)
	d.AddHandler(voiceStateUpdate)
	d.AddHandler(relationshipAdd)
	d.AddHandler(relationshipRemove)
	d.AddHandler(guildMemberAdd)
	d.AddHandler(guildMemberUpdate)
	d.AddHandler(guildMemberRemove)

	if *debug {
		d.AddHandler(onTyping)

		d.AddHandler(func(s *discordgo.Session, r *discordgo.Resumed) {
			log.Println(spew.Sdump(r))
		})

		d.AddHandler(func(s *discordgo.Session, dc *discordgo.Disconnect) {
			log.Println(spew.Sdump(dc))
		})

		// d.AddHandler(func(s *discordgo.Session, i interface{}) {
		// 	log.Println(spew.Sdump(i))
		// })
	}

	// d.AddHandler(func(s *discordgo.Session, ev *discordgo.Event) {
	// 	log.Println(spew.Sdump(ev))
	// })

	d.StateEnabled = true
	d.State.MaxMessageCount = 35
	d.State.TrackChannels = true
	d.State.TrackEmojis = true
	d.State.TrackMembers = true
	d.State.TrackRoles = true
	d.State.TrackVoice = true
	d.State.TrackPresences = true

	if err := d.Open(); err != nil {
		panic(err)
	}

	defer d.Close()
	defer app.Stop()

	log.Println("Storing token inside keyring...")
	if err := keyring.Set(AppName, "token", d.Token); err != nil {
		log.Println("Failed to set keyring! Continuing anyway...", err.Error())
	}

	// Stored in syscall.go, only does something when target OS is Linux
	syscallSilenceStderr(logFile)

	if err := app.Run(); err != nil {
		log.Panicln(err)
	}
}
