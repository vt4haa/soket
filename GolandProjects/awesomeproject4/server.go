package main

import (
	"fmt"
	"net"
	"os"
)

func main() {
	// Создаем сервер, который прослушивает порт 8080
	listener, err := net.Listen("tcp", ":8080")
	if err != nil {
		fmt.Println("Ошибка при создании сервера:", err)
		return
	}
	defer listener.Close()
	fmt.Println("Сервер запущен и ожидает подключения...")

	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Println("Ошибка при подключении клиента:", err)
			continue
		}
		go handleConnection(conn)
	}
}

func handleConnection(conn net.Conn) {
	defer conn.Close()
	file, err := os.Create("received_file")
	if err != nil {
		fmt.Println("Ошибка создания файла:", err)
		return
	}
	defer file.Close()

	_, err = file.ReadFrom(conn)
	if err != nil {
		fmt.Println("Ошибка при чтении данных:", err)
	}
	fmt.Println("Файл успешно получен и сохранен.")
}
