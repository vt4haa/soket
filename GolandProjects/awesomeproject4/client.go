package main

import (
	"fmt"
	"net"
	"os"
)

func main() {
	conn, err := net.Dial("tcp", "localhost:8080")
	if err != nil {
		fmt.Println("Ошибка подключения к серверу:", err)
		return
	}
	defer conn.Close()

	file, err := os.Open("file_to_send")
	if err != nil {
		fmt.Println("Ошибка открытия файла:", err)
		return
	}
	defer file.Close()

	_, err = file.WriteTo(conn)
	if err != nil {
		fmt.Println("Ошибка отправки файла:", err)
	}
	fmt.Println("Файл успешно отправлен.")
}
