package main

import (
	"fmt"
	"html/template"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"awesomeproject4/chat"
)

var sessions = make(map[string]string)
var mutex sync.Mutex

// MemoryRepo — простое in-memory хранилище сообщений для демонстрации
type MemoryRepo struct {
	mu             sync.Mutex
	roomToMessages map[string][]chat.Message
}

func NewMemoryRepo() *MemoryRepo {
	return &MemoryRepo{roomToMessages: make(map[string][]chat.Message)}
}

func (m *MemoryRepo) SaveMessage(msg chat.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.roomToMessages[msg.Room] = append(m.roomToMessages[msg.Room], msg)
	return nil
}

func (m *MemoryRepo) GetLastMessages(room string, count int) ([]chat.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	messages := m.roomToMessages[room]
	if count <= 0 || count >= len(messages) {
		return append([]chat.Message(nil), messages...), nil
	}
	return append([]chat.Message(nil), messages[len(messages)-count:]...), nil
}

func main() {
	http.HandleFunc("/", loginHandler)
	http.HandleFunc("/upload", uploadHandler)
	http.HandleFunc("/files", listFilesHandler)
	http.HandleFunc("/download/", downloadHandler)
	fs := http.FileServer(http.Dir("./static"))
	http.Handle("/static/", http.StripPrefix("/static/", fs))

	// Инициализация чата (WebSocket)
	repo := NewMemoryRepo()
	hub := chat.NewHub(repo)
	go hub.Run()
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) { chat.ServeWS(hub, w, r) })

	fmt.Println("Сервер запущен на http://localhost:8080")
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		fmt.Println("Ошибка запуска сервера:", err)
	}
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		tmpl, err := template.ParseFiles("templates/login.html")
		if err != nil {
			http.Error(w, "Ошибка загрузки страницы", http.StatusInternalServerError)
			return
		}
		tmpl.Execute(w, nil)
	} else if r.Method == "POST" {
		userID := r.FormValue("username")
		sessionToken := fmt.Sprintf("%d", time.Now().UnixNano())

		mutex.Lock()
		sessions[sessionToken] = userID
		mutex.Unlock()

		http.SetCookie(w, &http.Cookie{
			Name:  "session_token",
			Value: sessionToken,
			Path:  "/",
		})
		http.Redirect(w, r, "/upload", http.StatusFound)
	}
}

func uploadHandler(w http.ResponseWriter, r *http.Request) {
	sessionToken, err := r.Cookie("session_token")
	if err != nil {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	userID := sessions[sessionToken.Value]

	if r.Method == "GET" {
		tmpl, err := template.ParseFiles("templates/upload.html")
		if err != nil {
			http.Error(w, "Ошибка загрузки страницы", http.StatusInternalServerError)
			return
		}
		tmpl.Execute(w, userID)
	} else if r.Method == "POST" {
		file, handler, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "Ошибка при загрузке файла", http.StatusInternalServerError)
			return
		}
		defer file.Close()

		userDir := filepath.Join("uploads", userID)
		os.MkdirAll(userDir, os.ModePerm)

		dst, err := os.Create(filepath.Join(userDir, handler.Filename))
		if err != nil {
			http.Error(w, "Ошибка создания файла на сервере", http.StatusInternalServerError)
			return
		}
		defer dst.Close()

		_, err = io.Copy(dst, file)
		if err != nil {
			http.Error(w, "Ошибка сохранения файла", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		jsonResponse := fmt.Sprintf(`{"message": "Файл успешно загружен: %s"}`, handler.Filename)
		w.Write([]byte(jsonResponse))
	}
}

func listFilesHandler(w http.ResponseWriter, r *http.Request) {
	sessionToken, err := r.Cookie("session_token")
	if err != nil {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	userID := sessions[sessionToken.Value]
	userDir := filepath.Join("uploads", userID)

	files, err := os.ReadDir(userDir)
	if err != nil {
		http.Error(w, "Ошибка чтения каталога файлов", http.StatusInternalServerError)
		return
	}

	for _, file := range files {
		fmt.Fprintf(w, "<a href='/download/%s/%s'>%s</a><br>", userID, file.Name(), file.Name())
	}
}

func downloadHandler(w http.ResponseWriter, r *http.Request) {
	fileName := filepath.Base(r.URL.Path)
	userDir := filepath.Dir(r.URL.Path)
	filePath := filepath.Join(userDir, fileName)

	file, err := os.Open(filePath)
	if err != nil {
		http.Error(w, "Файл не найден", http.StatusNotFound)
		return
	}
	defer file.Close()

	w.Header().Set("Content-Disposition", "attachment; filename="+fileName)
	w.Header().Set("Content-Type", "application/octet-stream")
	io.Copy(w, file)
}