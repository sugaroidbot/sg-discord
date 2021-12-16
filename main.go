package main

import (
	"fmt"
	"os/signal"
	"syscall"

	"github.com/bwmarrin/discordgo"
	"github.com/google/uuid"
	"github.com/sugaroidbot/sg-telegram/sgapi"
	"github.com/withmandala/go-log"

	"os"
	"strings"
)

var logger = log.New(os.Stdout)

var chanMap = map[string]*sgapi.WsConn{}

const discordMessageLimit = 1750

func main() {
	dsBotToken := os.Getenv("DISCORD_BOT_TOKEN")
	wsEndpoint := os.Getenv("SG_DS_WS_ENDPOINT")

	dg, err := discordgo.New("Bot " + dsBotToken)
	if err != nil {
		logger.Fatal("error creating Discord session,", err)
		return
	}
	userId, err := dg.User("@me")
	if err != nil {
		panic(err)
	}
	prefix := fmt.Sprintf("<@!%s>", userId.ID)

	dg.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		onMessageReceiveHandler(s, m, prefix, wsEndpoint)
	})
	dg.Identify.Intents = discordgo.IntentsGuildMessages
	err = dg.Open()
	if err != nil {
		logger.Fatal(err)
		return
	}

	// Wait here until CTRL-C or other term signal is received.
	fmt.Println("Bot is now running.  Press CTRL-C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)
	<-sc

	dg.Close()
}

func sendMessageAsChunks(s *discordgo.Session, message string, chanId string) {
	if len(message) > discordMessageLimit {
		_, err := s.ChannelMessageSend(chanId, message[:discordMessageLimit]+"...")
		if err != nil {
			logger.Warn(err)
		}
		message = message[discordMessageLimit:]
		if len(message) > 0 {
			sendMessageAsChunks(s, message, chanId)
		}
	} else {
		s.ChannelMessageSend(chanId, message)
	}

}

// This function will be called (due to AddHandler above) every time a new
// message is created on any channel that the authenticated bot has access to.
func onMessageReceiveHandler(s *discordgo.Session, m *discordgo.MessageCreate, prefix string, wsEndpoint string) {

	// Ignore all messages created by the bot itself
	// This isn't required in this specific example but it's a good practice.
	if m.Author.ID == s.State.User.ID {
		return
	}
	fmt.Println(m.Content, prefix)
	if !strings.HasPrefix(m.Content, prefix) && !strings.HasPrefix(m.Content, "!S") {
		return
	}

	s.ChannelTyping(m.ChannelID)
	v, ok := chanMap[m.ChannelID]
	if !ok || v == nil {
		uid := uuid.New()
		scheme := "wss"

		// use ws:// for localhost, and similar ones
		if strings.HasPrefix(wsEndpoint, "0.0.0.0") || strings.HasPrefix(wsEndpoint, "127.0.0.1") || strings.HasPrefix(wsEndpoint, "localhost") {
			scheme = "ws"
		}
		wsCon, err := sgapi.New(sgapi.Instance{Endpoint: fmt.Sprintf("%s://%s", scheme, wsEndpoint)}, uid)
		if err != nil {
			logger.Warn(err)
			return
		}
		v = wsCon
		chanMap[m.ChannelID] = wsCon

		go func() {
			err := sgapi.Listen(wsCon, func(resp string) {
				if resp == "" {
					// skip empty responses
					return
				}
				sendMessageAsChunks(s, resp, m.ChannelID)
				/*
					msg := tgbotapi.NewMessage(Chat.ID, resp)

					if strings.Contains(resp, "<sugaroid:yesno>") {
						msg.Text = strings.Replace(resp, "<sugaroid:yesno>", "", -1)
						msg.ReplyMarkup = keyboards["sugaroid:yesno"]
					}
					msg.ParseMode = tgbotapi.ModeHTML

					_, err := bot.Send(msg)
					if err != nil {
						logger.Warn(err)
					}*/
			})
			if err != nil {
				logger.Warn(err)
				chanMap[m.ChannelID] = nil
				return
			}
		}()
	}

	logger.Infof("[#%s][%s] %s", m.ChannelID, m.Author.ID, m.Content)

	err := sgapi.Send(v, strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(m.Content, prefix), "!S ")))
	if err != nil {
		logger.Warn(err)
		chanMap[m.ChannelID] = nil
		return
	}
}
