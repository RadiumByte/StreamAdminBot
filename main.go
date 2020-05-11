package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"reflect"
	"strconv"
	"time"

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

// AddCameraData discribes new camera data
type AddCameraData struct {
	Name string `json:"name"`
	Type int    `json:"type"`
	URL  string `json:"url"`
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
	presets []AddCameraData

	newCamera AddCameraData
)

func haltSystem() {
	isAwake = false

	cmdKill := exec.Command("killall", "StreamServer")
	cmdKill.Run()

	cmdKill = exec.Command("killall", "/home/anton/Radium/LabYoutubeChatbot/LabYoutubeChatbot")
	cmdKill.Run()
}

func awakeSystem() {
	if !isAwake {
		client = &fasthttp.Client{}
		request = fasthttp.AcquireRequest()
		response = fasthttp.AcquireResponse()
		serverPort = ":8081"
		serverIP = "localhost"

		cmdRunServer := exec.Command("/home/anton/Radium/StreamServer/StreamServer")
		cmdRunServer.Start()

		time.Sleep(5 * time.Second)

		cmdRunChatbot := exec.Command("/home/anton/Radium/LabYoutubeChatbot/LabYoutubeChatbot")
		cmdRunChatbot.Start()

		isAwake = true
	}
}

func isAdmin(id int) bool {
	for _, item := range adminID {
		if item == id {
			return true
		}
	}
	return false
}

func isNameUnique(name string) bool {
	for _, item := range cameras {
		if item.Name == name {
			return false
		}
	}
	return true
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

func setupPresets() {
	corridor := AddCameraData{
		Name: "Коридор",
		URL:  "rtsp://192.168.1.223:554/user=admin_password=tlJwpbo6_channel=1_stream=0.sdp?real_stream",
		Type: 1}
	presets = append(presets, corridor)

	webcam := AddCameraData{
		Name: "Вебка ноута",
		URL:  "/dev/video0",
		Type: 0}
	presets = append(presets, webcam)
}

// getCameras receives list of all available cameras from Stream Server
func getCameras() ([]CameraData, error) {
	request.Header.SetMethod("GET")

	url := "http://" + serverIP + serverPort + "/get-cameras"

	request.SetRequestURI(url)
	err := client.Do(request, response)
	log.Printf("Status code: %d\n", response.StatusCode())

	if response.StatusCode() == 204 || response.StatusCode() == 400 {
		return nil, nil
	}

	if err != nil {
		fmt.Println("Client: GetCameras failed to make a request.")
		return nil, err
	}

	payload := response.Body()
	var dataJSON map[string]interface{}

	if err := json.Unmarshal(payload, &dataJSON); err != nil {
		fmt.Println("Client: Server returned bad data for GetCameras")
		return nil, err
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

	return cameras, nil
}

// getActive gets one active (broadcasting) camera at this moment
func getActive() (CameraData, error) {
	request.Header.SetMethod("GET")

	url := "http://" + serverIP + serverPort + "/get-active"

	request.SetRequestURI(url)
	err := client.Do(request, response)
	log.Printf("Status code: %d\n", response.StatusCode())

	if response.StatusCode() == 204 || response.StatusCode() == 400 {
		return CameraData{}, nil
	}

	if err != nil {
		fmt.Println("Client: GetActive failed to make a request.")
		return CameraData{}, err
	}

	payload := response.Body()
	var dataJSON map[string]interface{}

	if err := json.Unmarshal(payload, &dataJSON); err != nil {
		fmt.Println("Client: Server returned bad data for GetActive")
		return CameraData{}, err
	}

	typeCam := int(dataJSON["type"].(float64))
	nameCam := dataJSON["name"].(string)

	return CameraData{
		Name: nameCam,
		Type: typeCam}, nil
}

func addCamera(data AddCameraData) error {
	request.Header.SetMethod("POST")
	request.Header.SetContentType("application/json")

	url := "http://" + serverIP + serverPort + "/add-camera"
	request.SetRequestURI(url)

	payload, _ := json.Marshal(data)

	request.SetBody(payload)

	err := client.Do(request, response)
	log.Printf("Status code: %d\n", response.StatusCode())

	if err != nil {
		return err
	}
	return nil
}

// selectCamera makes specified camera active, switching the broadcast
func selectCamera(name string) error {
	request.Header.SetMethod("POST")
	request.Header.SetContentType("application/json")

	url := "http://" + serverIP + serverPort + "/select-camera"
	request.SetRequestURI(url)

	toEncode := &SelectCameraJSON{
		CameraName: name}

	payload, _ := json.Marshal(toEncode)

	request.SetBody(payload)

	err := client.Do(request, response)
	log.Printf("Status code: %d\n", response.StatusCode())

	if err != nil {
		return err
	}
	return nil
}

func main() {
	newCamera = AddCameraData{}
	setupPresets()

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
			log.Printf("Current state: %d", int(currentState))

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
					awakeSystem()
					message := "Система запущена."

					msg := tgbotapi.NewMessage(update.Message.Chat.ID, message)
					bot.Send(msg)
					currentState = StateWork

				case "/halt":
					haltSystem()
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
						var err error
						cameras, err = getCameras()
						if err != nil {
							message := "Сервер не отвечает, проверьте его состояние."
							msg := tgbotapi.NewMessage(update.Message.Chat.ID, message)
							bot.Send(msg)
							currentState = StateWork
							continue
						}

						message := ""

						if len(cameras) != 0 {
							message = "Список доступных камер:\n"
							for i := 0; i < len(cameras); i++ {
								data := strconv.Itoa(i+1) + ") " + cameras[i].Name + " ("
								if cameras[i].Type == 1 || cameras[i].Type == 2 {
									data += "RTSP)"
								} else {
									data += "Webcam)"
								}
								message += data + "\n"
							}
							message += "\n"
						} else {
							message = "Сейчас нет доступных камер. Вы можете выбрать готовую камеру /addpreset или создать новую с нуля /addcamera."
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
						cam, err := getActive()
						if err != nil {
							message := "Сервер не отвечает, проверьте его состояние."
							msg := tgbotapi.NewMessage(update.Message.Chat.ID, message)
							bot.Send(msg)
							currentState = StateWork
							continue
						}

						message := ""

						if cam.Name != "" {
							message = "Камера, с которой ведется трансляция:\n"
							message += cam.Name + " ("
							if cam.Type == 1 || cam.Type == 2 {
								message += "RTSP)"
							} else {
								message += "Webcam)"
							}
						} else {
							message = "Сейчас ни одна камера не работает. Вы можете выбрать готовую камеру /addpreset или создать новую с нуля /addcamera."
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
						var err error
						cameras, err = getCameras()
						if err != nil {
							message := "Сервер не отвечает, проверьте его состояние."
							msg := tgbotapi.NewMessage(update.Message.Chat.ID, message)
							bot.Send(msg)
							currentState = StateWork
							continue
						}

						message := ""

						if len(cameras) != 0 {
							message = "Список доступных камер:\n"

							for i := 0; i < len(cameras); i++ {
								data := strconv.Itoa(i+1) + ") " + cameras[i].Name + " ("
								if cameras[i].Type == 1 || cameras[i].Type == 2 {
									data += "RTSP)"
								} else {
									data += "Webcam)"
								}
								message += data + "\n"
							}
							message += "\n"
							message += "Сделайте выбор, введя номер камеры в списке, например, 1 или 2. Для отмены введите /cancel."
							currentState = StateSelectCamera
						} else {
							message = "Сейчас нет доступных камер. Вы можете выбрать готовую камеру /addpreset или создать новую с нуля /addcamera."
							currentState = StateWork
						}

						msg := tgbotapi.NewMessage(update.Message.Chat.ID, message)
						bot.Send(msg)
					}

				case "/addcamera":
					if !isAwake {
						message := "Важно - настройка камер невозможна при выключенной системе. Пожалуйста, выполните команду /awake для запуска.\n"
						msg := tgbotapi.NewMessage(update.Message.Chat.ID, message)
						bot.Send(msg)
					} else {
						newCamera.Name = ""
						newCamera.Type = -1
						newCamera.URL = ""

						message := "Введите уникальное имя новой камеры:"

						msg := tgbotapi.NewMessage(update.Message.Chat.ID, message)
						bot.Send(msg)
						currentState = StateEnterName
					}

				case "/addpreset":
					if !isAwake {
						message := "Важно - настройка камер невозможна при выключенной системе. Пожалуйста, выполните команду /awake для запуска.\n"
						msg := tgbotapi.NewMessage(update.Message.Chat.ID, message)
						bot.Send(msg)
					} else {
						newCamera.Name = ""
						newCamera.Type = -1
						newCamera.URL = ""

						message := "Список доступных пресетов:\n"

						for i := 0; i < len(presets); i++ {
							data := strconv.Itoa(i+1) + ") " + presets[i].Name + " ("
							if presets[i].Type == 1 || presets[i].Type == 2 {
								data += "RTSP)"
							} else {
								data += "Webcam)"
							}
							message += data + "\n"
						}
						message += "\n"
						message += "Сделайте выбор, введя номер камеры в списке, например, 1 или 2. Для отмены введите /cancel."

						msg := tgbotapi.NewMessage(update.Message.Chat.ID, message)
						bot.Send(msg)
						currentState = StateSelectPreset
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

						selectCamera(cameras[value-1].Name)
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
					if value, err := strconv.ParseInt(update.Message.Text, 10, 64); err == nil {
						if value < 1 || value > int64(len(presets)) {
							message := "Простите, но камеры с таким номером не существует. Введите другой номер или /cancel."
							msg := tgbotapi.NewMessage(update.Message.Chat.ID, message)
							bot.Send(msg)
							currentState = StateSelectPreset
							continue
						}

						newCamera.Name = presets[value-1].Name
						newCamera.Type = presets[value-1].Type
						newCamera.URL = presets[value-1].URL

						err := addCamera(newCamera)
						if err != nil {
							message := "Сервер не отвечает, проверьте его состояние."
							msg := tgbotapi.NewMessage(update.Message.Chat.ID, message)
							bot.Send(msg)
							currentState = StateWork
							continue
						}

						message := "Новая камера успешно создана. Вы можете ее увидеть в списке, введя команду /getcameras."
						msg := tgbotapi.NewMessage(update.Message.Chat.ID, message)
						bot.Send(msg)
						currentState = StateWork
					}
				}

			case StateEnterName:
				if update.Message.Text == "/cancel" {
					message := "Создание новой камеры отменено. Введите следующую команду."
					msg := tgbotapi.NewMessage(update.Message.Chat.ID, message)
					bot.Send(msg)
					currentState = StateWork
				} else {
					if !isNameUnique(update.Message.Text) {
						message := "Данное имя камеры уже занято. Пожалуйста, введите другое имя."
						msg := tgbotapi.NewMessage(update.Message.Chat.ID, message)
						bot.Send(msg)
						currentState = StateEnterName
						continue
					}

					newCamera.Name = update.Message.Text

					message := "Введите число от 0 до 2, описывающее тип новой камеры:\n"
					message += "0 - USB-камера, подключенная к серверу;\n"
					message += "1 - RTSP-камера, использующая протокол TCP;\n"
					message += "2 - RTSP-камера, использующая протокол UDP."

					msg := tgbotapi.NewMessage(update.Message.Chat.ID, message)
					bot.Send(msg)
					currentState = StateEnterType
				}

			case StateEnterType:
				if update.Message.Text == "/cancel" {
					message := "Создание новой камеры отменено. Введите следующую команду."
					msg := tgbotapi.NewMessage(update.Message.Chat.ID, message)
					bot.Send(msg)
					currentState = StateWork
				} else {
					if value, err := strconv.ParseInt(update.Message.Text, 10, 64); err == nil {
						if value < 0 || value > 2 {
							message := "Простите, но такого типа камер не существует. Введите другой тип или /cancel."
							msg := tgbotapi.NewMessage(update.Message.Chat.ID, message)
							bot.Send(msg)
							currentState = StateEnterType
							continue
						}

						newCamera.Type = int(value)

						message := ""

						if newCamera.Type == 0 {
							message = "Введите номер video-устройства, подключенного к серверу:\n"
						} else {
							message = "Введите полную строку подключения к RTSP камере (зависит от ее производителя), например:\n"
							message += "rtsp://192.168.1.2:554/user=admin_password=abcdef_channel=1_stream=0.sdp?real_stream"
						}

						msg := tgbotapi.NewMessage(update.Message.Chat.ID, message)
						bot.Send(msg)
						currentState = StateEnterURL
					}
				}

			case StateEnterURL:
				if update.Message.Text == "/cancel" {
					message := "Создание новой камеры отменено. Введите следующую команду."
					msg := tgbotapi.NewMessage(update.Message.Chat.ID, message)
					bot.Send(msg)
					currentState = StateWork
				} else {
					if newCamera.Type == 0 {
						if value, err := strconv.ParseInt(update.Message.Text, 10, 64); err == nil {
							if value < 0 {
								message := "Простите, но такого номера камер не существует. Введите другой номер или /cancel."
								msg := tgbotapi.NewMessage(update.Message.Chat.ID, message)
								bot.Send(msg)
								currentState = StateEnterURL
								continue
							}
							newCamera.URL = "/dev/video" + strconv.Itoa(int(value))
						}
					} else {
						newCamera.URL = update.Message.Text
					}

					err := addCamera(newCamera)
					if err != nil {
						message := "Сервер не отвечает, проверьте его состояние."
						msg := tgbotapi.NewMessage(update.Message.Chat.ID, message)
						bot.Send(msg)
						currentState = StateWork
						continue
					}

					message := "Новая камера успешно создана. Вы можете ее увидеть в списке, введя команду /getcameras."
					msg := tgbotapi.NewMessage(update.Message.Chat.ID, message)
					bot.Send(msg)
					currentState = StateWork
				}
			}
		}
	}
}
