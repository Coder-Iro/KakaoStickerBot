package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"
	"golang.org/x/image/draw"
)

var urlRegex = regexp.MustCompile("https://e.kakao.com/t/.+")

type Result struct {
	Title         string   `json:"title"`
	ThumbnailUrls []string `json:"thumbnailUrls"`
}

type Meta struct {
	Result Result `json:"result"`
}

// This bot demonstrates some example interactions with commands on telegram.
// It has a basic start command with a bot intro.
// It also has a source command, which sends the bot sourcecode, as a file.
func main() {
	// Get token from the environment variable
	token := os.Getenv("TELEGRAM_TOKEN")
	if token == "" {
		panic("TELEGRAM_TOKEN environment variable is empty")
	}

	// Create bot from environment value.
	b, err := gotgbot.NewBot(token, nil)
	if err != nil {
		panic("Failed to create new bot: " + err.Error())
	}

	// Create updater and dispatcher.
	updater := ext.NewUpdater(&ext.UpdaterOpts{
		Dispatcher: ext.NewDispatcher(&ext.DispatcherOpts{
			// If an error is returned by a handler, log it and continue going.
			Error: func(b *gotgbot.Bot, ctx *ext.Context, err error) ext.DispatcherAction {
				log.Println("An error occurred while handling update:", err.Error())
				return ext.DispatcherActionNoop
			},
			MaxRoutines: ext.DefaultMaxRoutines,
		}),
	})
	dispatcher := updater.Dispatcher

	// /start command to introduce the bot
	dispatcher.AddHandler(handlers.NewCommand("start", start))
	dispatcher.AddHandler(handlers.NewCommand("help", start))
	// /source command to send the bot source code
	dispatcher.AddHandler(handlers.NewCommand("create", create))

	// Start receiving updates.
	err = updater.StartPolling(b, &ext.PollingOpts{
		DropPendingUpdates: true,
		GetUpdatesOpts: &gotgbot.GetUpdatesOpts{
			Timeout: 9,
			RequestOpts: &gotgbot.RequestOpts{
				Timeout: time.Second * 10,
			},
		},
	})
	if err != nil {
		panic("Failed to start polling: " + err.Error())
	}
	log.Printf("%s has been started...\n", b.User.Username)

	// Idle, to keep updates coming in, and avoid bot stopping.
	updater.Idle()
}

func create(b *gotgbot.Bot, ctx *ext.Context) error {

	if len(ctx.Args()) == 2 && urlRegex.MatchString(ctx.Args()[1]) {
		ctx.EffectiveMessage.Reply(b, "이모티콘 정보를 불러오는 중입니다.", nil)
		url := strings.ReplaceAll(ctx.Args()[1], "https://e.kakao.com/t/", "https://e.kakao.com/api/v1/items/t/")
		resp, err := http.Get(url)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}

		emoticonMeta := Meta{}

		jsonErr := json.Unmarshal(body, &emoticonMeta)
		if jsonErr != nil {
			return jsonErr
		}
		b.SendMessage(ctx.EffectiveChat.Id, fmt.Sprintf("<b>%s</b> 이모티콘을 다운로드 합니다.", emoticonMeta.Result.Title), &gotgbot.SendMessageOpts{
			ParseMode: "html",
		})

		rect := image.Rect(0, 0, 512, 512)
		stickers := []gotgbot.InputSticker{}
		dwmsg, _ := b.SendMessage(ctx.EffectiveChat.Id, fmt.Sprintf("다운로드 중... <b>(0/%d)</b>", len(emoticonMeta.Result.ThumbnailUrls)), &gotgbot.SendMessageOpts{
			ParseMode: "html",
		})
		for index, thumb := range emoticonMeta.Result.ThumbnailUrls {
			resp, err := http.Get(thumb)
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			img, _, err := image.Decode(resp.Body)
			if err != nil {
				return err
			}
			buf := bytes.Buffer{}
			dst := image.NewNRGBA(rect)
			draw.ApproxBiLinear.Scale(dst, rect, img, img.Bounds(), draw.Over, nil)
			png.Encode(&buf, dst)
			stickers = append(stickers, gotgbot.InputSticker{
				Sticker:   bytes.NewReader(buf.Bytes()),
				EmojiList: []string{"😀"},
			})
			dwmsg.EditText(b, fmt.Sprintf("다운로드 중... <b>(%d/%d)</b>", index+1, len(emoticonMeta.Result.ThumbnailUrls)), &gotgbot.EditMessageTextOpts{
				ParseMode: "html",
			})
		}

		b.SendMessage(ctx.EffectiveChat.Id, fmt.Sprintf("총 <b>%d</b> 개의 이모티콘을 텔레그램 서버로 업로드합니다.", len(emoticonMeta.Result.ThumbnailUrls)), &gotgbot.SendMessageOpts{
			ParseMode: "html",
		})

		upmsg, _ := b.SendMessage(ctx.EffectiveChat.Id, fmt.Sprintf("업로드 중... <b>(0/%d)</b>", len(emoticonMeta.Result.ThumbnailUrls)), &gotgbot.SendMessageOpts{
			ParseMode: "html",
		})

		stickerSet := fmt.Sprintf("t%d_by_%s", time.Now().UnixNano(), b.Username)

		_, createErr := b.CreateNewStickerSet(
			ctx.EffectiveSender.Id(),
			stickerSet,
			emoticonMeta.Result.Title,
			[]gotgbot.InputSticker{stickers[0]},
			"static",
			nil,
		)
		upmsg.EditText(b, fmt.Sprintf("업로드 중... <b>(1/%d)</b>", len(emoticonMeta.Result.ThumbnailUrls)), &gotgbot.EditMessageTextOpts{
			ParseMode: "html",
		})

		for index, sticker := range stickers[1:] {
			b.AddStickerToSet(ctx.EffectiveSender.Id(), stickerSet, sticker, nil)
			upmsg.EditText(b, fmt.Sprintf("업로드 중... <b>(%d/%d)</b>", index+2, len(emoticonMeta.Result.ThumbnailUrls)), &gotgbot.EditMessageTextOpts{
				ParseMode: "html",
			})
		}

		if createErr != nil {
			return createErr
		}

		b.SendMessage(ctx.EffectiveChat.Id, fmt.Sprintf("<b>%s</b> 스티커 생성이 완료되었습니다!\nhttps://t.me/addstickers/%s", emoticonMeta.Result.Title, stickerSet), &gotgbot.SendMessageOpts{
			ParseMode: "html",
		})

	} else {
		ctx.EffectiveMessage.Reply(b, "유효한 이모티콘 URL이 아닙니다.", nil)
	}

	return nil
}

// start introduces the bot.
func start(b *gotgbot.Bot, ctx *ext.Context) error {
	_, err := ctx.EffectiveMessage.Reply(b,
		"이모티콘을 스티커로 변환하시려면 /create [이모티콘URL] 을 입력해주세요. 웹 버전 이모티콘 스토어 URL만 가능합니다.", nil)
	if err != nil {
		return fmt.Errorf("failed to send start message: %w", err)
	}
	return nil
}
