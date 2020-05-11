package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"strconv"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"github.com/valyala/fasthttp"
	"golang.org/x/net/proxy"
)

// State describes state of chatbot.
type State int

// Status.
const (
	StateWork         State = 1
	StateSelectCamera State = 2
	StateSelectPreset State = 3
	StateEnterType    State = 4
	StateEnterURL     State = 5
	StateEnterName    State = 6
)

// CameraData discribes generic data
type CameraData struct {
	Name string
	Type int
}

// SelectCameraJSON represents transport data for camera switching
type SelectCameraJSON struct {
	CameraName string `json:"name"`
}

var (
	adminID    [1]int = [1]int{634596120}
	isAwake    bool   = false
	client     *fasthttp.Client
	request    *fasthttp.Request
	response   *fasthttp.Response
	serverIP   string
	serverPort string

	currentState State = StateWork

	cameras []CameraData
)

func haltSystem() {
	isAwake = false
}

func awakeSystem() {
	isAwake = true

	client = &fasthttp.Client{}
	request = fasthttp.AcquireRequest()
	response = fasthttp.AcquireResponse()
	serverPort = ":8081"
	serverIP = "localhost"
}

func isAdmin(id int) bool {
	for _, item := range adminID {
		if item == id {
			return true
		}
	}
	return false
}

func helpMessage() string {
	message := ""
	message += "Важно - настройка камер невозможна при выключенной системе. Пожалуйста, выполните команду /awake для запуска.\n\n"
	message += "Введите одну из команд:\n\n"
	message += "Настройка камер\n"
	message += "/getcameras - получить список камер\n"
	message += "/getactive - посмотреть текущую выбранную камеру\n"
	message += "/selectcamera - выбрать камеру\n"
	message += "/addcamera - добавить новую камеру\n"
	message += "/addpreset - добавить готовую камеру\n\n"
	message += "Общее\n"
	message += "/awake - запустить систему трансляций\n"
	message += "/halt - выключить систему трансляций\n"
	message += "/help - помощь по командам\n"
	return message
}

// getCameras receives list of all available cameras from Stream Server
func getCameras() []CameraData {
	request.Header.SetMethod("GET")

	url := "http://" + serverIP + serverPort + "/get-cameras"

	request.SetRequestURI(url)
	err := client.Do(request, response)

	if err != nil {
		fmt.Println("Client: GetCameras failed to make a request.")
		return nil
	}

	payload := response.Body()
	var dataJSON map[string]interface{}

	if err := json.Unmarshal(payload, &dataJSON); err != nil {
		fmt.Println("Client: Server returned bad data for GetCameras")
		return nil
	}

	var types []interface{}
	var names []interface{}
	var cameras []CameraData

	types = dataJSON["types"].([]interface{})
	names = dataJSON["names"].([]interface{})

	for i := 0; i < len(types); i++ {
		current := CameraData{
			Name: names[i].(string),
			Type: int(types[i].(float64))}
		cameras = append(cameras, current)
	}

	return cameras
}

// getActive gets one active (broadcasting) camera at this moment
func getActive() CameraData {
	request.Header.SetMethod("GET")

	url := "http://" + serverIP + serverPort + "/get-active"

	request.SetRequestURI(url)
	err := client.Do(request, response)

	if err != nil {
		fmt.Println("Client: GetActive failed to make a request.")
		return CameraData{}
	}

	payload := response.Body()
	var dataJSON map[string]interface{}

	if err := json.Unmarshal(payload, &dataJSON); err != nil {
		fmt.Println("Client: Server returned bad data for GetActive")
		return CameraData{}
	}

	typeCam := int(dataJSON["type"].(float64))
	nameCam := dataJSON["name"].(string)

	return CameraData{
		Name: nameCam,
		Type: typeCam}
}

// selectCamera makes specified camera active, switching the broadcast
func selectCamera(name string) {
	request.Header.SetMethod("POST")
	request.Header.SetContentType("application/json")

	url := "http://" + serverIP + serverPort + "/select-camera"
	request.SetRequestURI(url)

	toEncode := &SelectCameraJSON{
		CameraName: name}

	payload, _ := json.Marshal(toEncode)

	request.SetBody(payload)

	client.Do(request, response)
}

func main() {
	socks5 := os.Getenv("SOCKS5_PROXY")
	client := &http.Client{}

	if len(socks5) > 0 {
		log.Printf("Found SOCKS5 PROXY: %s\n", socks5)
		tgProxyURL, err := url.Parse(socks5)
		if err != nil {
			log.Printf("Failed to parse proxy URL:%s\n", err)
			log.Panic(err)
		}
		tgDialer, err := proxy.FromURL(tgProxyURL, proxy.Direct)
		if err != nil {
			log.Printf("Failed to obtain proxy dialer: %s\n", err)
		}
		tgTransport := &http.Transport{
			Dial: tgDialer.Dial,
		}
		client.Transport = tgTransport
	}

	bot, err := tgbotapi.NewBotAPIWithClient("", tgbotapi.APIEndpoint, client)
	if err != nil {
		log.Panic(err)
	}

	bot.Debug = false

	log.Printf("Authorized on account %s", bot.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates, err := bot.GetUpdatesChan(u)

	for update := range updates {
		if update.Message == nil {
			continue
		}

		if reflect.TypeOf(update.Message.Text).Kind() == reflect.String && update.Message.Text != "" {
			log.Printf("[%d] %s", update.Message.From.ID, update.Message.Text)

			if !isAdmin(update.Message.From.ID) {
				log.Println("Unauthorized connection to the chatbot")
				msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Вы не авторизованы.\nПожалуйста, свяжитесь с администратором, чтобы получить права доступа: anton.fedyashov@gmail.com.")
				bot.Send(msg)
				continue
			}

			switch currentState {
			case StateWork:
				switch update.Message.Text {
				case "/start":
					message := "Привет! Я могу управлять системой онлайн-трансляций.\n"
					message += helpMessage()

					msg := tgbotapi.NewMessage(update.Message.Chat.ID, message)
					bot.Send(msg)
					currentState = StateWork

				case "/help":
					message := helpMessage()

					msg := tgbotapi.NewMessage(update.Message.Chat.ID, message)
					bot.Send(msg)
					currentState = StateWork

				case "/awake":
					message := "Система запущена."

					msg := tgbotapi.NewMessage(update.Message.Chat.ID, message)
					bot.Send(msg)
					currentState = StateWork

				case "/halt":
					message := "Система остановлена."

					msg := tgbotapi.NewMessage(update.Message.Chat.ID, message)
					bot.Send(msg)
					currentState = StateWork

				case "/getcameras":
					if !isAwake {
						message := "Важно - настройка камер невозможна при выключенной системе. Пожалуйста, выполните команду /awake для запуска.\n"
						msg := tgbotapi.NewMessage(update.Message.Chat.ID, message)
						bot.Send(msg)
					} else {
						cameras = getCameras()
						message := "Список доступных камер:\n"

						for i := 0; i < len(cameras); i++ {
							data := strconv.Itoa(i+1) + ") " + cameras[i].Name + " ("
							if cameras[i].Type == 1 {
								message += "RTSP)"
							} else {
								message += "Webcam)"
							}
							message += data + "\n"
						}
						msg := tgbotapi.NewMessage(update.Message.Chat.ID, message)
						bot.Send(msg)
						currentState = StateWork
					}

				case "/getactive":
					if !isAwake {
						message := "Важно - настройка камер невозможна при выключенной системе. Пожалуйста, выполните команду /awake для запуска.\n"
						msg := tgbotapi.NewMessage(update.Message.Chat.ID, message)
						bot.Send(msg)
					} else {
						cam := getActive()
						message := "Камера, с которой ведется трансляция:\n"
						message += cam.Name + " ("
						if cam.Type == 1 {
							message += "RTSP)"
						} else {
							message += "Webcam)"
						}

						msg := tgbotapi.NewMessage(update.Message.Chat.ID, message)
						bot.Send(msg)
						currentState = StateWork
					}

				case "/selectcamera":
					if !isAwake {
						message := "Важно - настройка камер невозможна при выключенной системе. Пожалуйста, выполните команду /awake для запуска.\n"
						msg := tgbotapi.NewMessage(update.Message.Chat.ID, message)
						bot.Send(msg)
					} else {
						cameras = getCameras()
						message := "Список доступных камер:\n"

						for i := 0; i < len(cameras); i++ {
							data := strconv.Itoa(i+1) + ") " + cameras[i].Name + " ("
							if cameras[i].Type == 1 {
								message += "RTSP)"
							} else {
								message += "Webcam)"
							}
							message += data + "\n"
						}
						message += "Сделайте выбор, введя номер камеры в списке (например, 1 или 2). Для отмены введите /cancel."

						msg := tgbotapi.NewMessage(update.Message.Chat.ID, message)
						bot.Send(msg)
						currentState = StateSelectCamera
					}
				}

			case StateSelectCamera:
				if update.Message.Text == "/cancel" {
					message := "Выбор камеры отменен. Введите следующую команду."
					msg := tgbotapi.NewMessage(update.Message.Chat.ID, message)
					bot.Send(msg)
					currentState = StateWork
				} else {
					if value, err := strconv.ParseInt(update.Message.Text, 10, 64); err == nil {
						if value < 1 || value > int64(len(cameras)) {
							message := "Простите, но камеры с таким номером не существует. Введите другой номер или /cancel."
							msg := tgbotapi.NewMessage(update.Message.Chat.ID, message)
							bot.Send(msg)
							currentState = StateSelectCamera
							continue
						}

						selectCamera(cameras[value].Name)
						message := "Камера успешно выбрана."
						msg := tgbotapi.NewMessage(update.Message.Chat.ID, message)
						bot.Send(msg)
						currentState = StateWork
					}
				}

			case StateSelectPreset:
				if update.Message.Text == "/cancel" {
					message := "Выбор готовой камеры отменен. Введите следующую команду."
					msg := tgbotapi.NewMessage(update.Message.Chat.ID, message)
					bot.Send(msg)
					currentState = StateWork
				} else {

				}

			case StateEnterName:
				if update.Message.Text == "/cancel" {
					message := "Создание новой камеры отменено. Введите следующую команду."
					msg := tgbotapi.NewMessage(update.Message.Chat.ID, message)
					bot.Send(msg)
					currentState = StateWork
				} else {

				}

			case StateEnterType:
				if update.Message.Text == "/cancel" {
					message := "Создание новой камеры отменено. Введите следующую команду."
					msg := tgbotapi.NewMessage(update.Message.Chat.ID, message)
					bot.Send(msg)
					currentState = StateWork
				} else {

				}

			case StateEnterURL:
				if update.Message.Text == "/cancel" {
					message := "Создание новой камеры отменено. Введите следующую команду."
					msg := tgbotapi.NewMessage(update.Message.Chat.ID, message)
					bot.Send(msg)
					currentState = StateWork
				} else {

				}

			}
		}
	}
}
