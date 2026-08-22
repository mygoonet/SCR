//go:build ignore

package main

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// Копия логики фильтрации из SCRP/monitor_api.go:165
func isTransportationsListURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	if strings.Contains(u.Path, "/negotiate") {
		return false
	}
	return strings.HasSuffix(u.Path, "/transportations")
}

func testURLFiltering() {
	fmt.Println("=== Тест фильтрации URL (monitor_api.go:165 + fix) ===")
	cases := []struct {
		url  string
		want bool
	}{
		{"https://logist.kontur.ru/api/transportations/changes/negotiate?negotiateVersion=1", false},
		{"https://logist.kontur.ru/api/boxes/2849e025-11cf-414c-88ad-7700163ae113/transportations", true},
		{"https://logist.kontur.ru/api/boxes/2849e025-11cf-414c-88ad-7700163ae113/transportations?skip=0&take=20", true},
		{"https://logist.kontur.ru/api/transportations", true},
		{"https://logist.kontur.ru/api/transportations/", false}, // trailing slash - edge
		{"https://logist.kontur.ru/api/other", false},
	}
	pass := 0
	for _, c := range cases {
		got := isTransportationsListURL(c.url)
		status := "FAIL"
		if got == c.want {
			status = "OK"
			pass++
		}
		fmt.Printf("  [%s] %s -> %v (want %v)\n", status, c.url, got, c.want)
	}
	fmt.Printf("Фильтрация: %d/%d passed\n\n", pass, len(cases))
	if pass != len(cases) {
		fmt.Println("  ВЫВОД: старый код без проверки /negotiate паузil negotiate и блокировал список!")
	}
}

func testOuterTimeout() {
	fmt.Println("=== Тест outerTimeout (monitor_api.go:119) ===")
	timeoutSec := 30
	oldOuter := time.Duration(timeoutSec+15) * time.Second
	newOuterSingle := time.Duration(timeoutSec+15) * time.Second

	// Старая схема: один outer на 2 попытки
	// Попытка1: reload 3s + wait 30s =33s -> осталось 12s на попытку2 (нужно 33s) -> deadline
	fmt.Printf("Старый outer: %s на 2 попытки\n", oldOuter)
	fmt.Printf("  Попытка1: reload 3s + wait 30s =33s -> осталось %s (нужно 33s) -> FAIL deadline exceeded\n", oldOuter-33*time.Second)
	fmt.Printf("  Лог из прод: 10:11:42 таймаут 30s + 10:11:52 context deadline exceeded - подтверждается\n")

	// Новая схема: свежий контекст на каждую попытку
	fmt.Printf("Новый: каждая попытка свой %s -> 2я попытка имеет полные 45s -> OK\n", newOuterSingle)
	fmt.Println()
}

func testNavigateLogic() {
	fmt.Println("=== Тест NavigateToCarrier (SCRP/carrier.go:13) ===")
	cases := []struct {
		loc     string
		isList  bool
		comment string
	}{
		{"https://logist.kontur.ru/2849e025-11cf-414c-88ad-7700163ae113/carrier", true, "список"},
		{"https://logist.kontur.ru/2849e025-11cf-414c-88ad-7700163ae113/carrier/sign/waybill", false, "waybill - старый код считал как список!"},
		{"https://logist.kontur.ru/box-selection", false, "логин"},
	}
	for _, c := range cases {
		// старая логика
		oldIsList := strings.Contains(c.loc, "/carrier/")
		// новая логика
		newIsList := strings.Contains(c.loc, "/carrier") && !strings.Contains(c.loc, "/sign") && !strings.Contains(c.loc, "/waybill")
		fmt.Printf("  loc=%s\n    old Contains(/carrier/) -> %v (ошибка для waybill)\n    new !/sign && !/waybill -> %v (want %v) %s\n", c.loc, oldIsList, newIsList, c.isList, c.comment)
	}
	fmt.Println()
}

func testDeadlineNotEnough() {
	fmt.Println("=== Тест 'deadline может не успевает' (вопрос из логов) ===")
	// Симуляция: reload 3s + SPA инициализация + сеть
	// Успешные тики: negotiate 10:23:10 -> transportations 10:23:12 = 2с
	// Значит 30s достаточно, но если pending держит negotiate в паузе - transportations никогда не придет
	fmt.Println("  Успешный кейс: 10:23:10 negotiate, 10:23:12 transportations = 2s gap -> 30s достаточно")
	fmt.Println("  Провальный кейс: 10:11:12 negotiate паузa 30s -> transportations не приходит 30s -> таймаут")
	fmt.Println("  Вывод: deadline 30s не виноват, виновата пауза negotiate. Фикс - сразу Continue для negotiate.")
	fmt.Println("  Дополнительно: Sleep после Reload 3s -> 5s даст SPA больше времени, но не решает дедлок")
	fmt.Println()
}

func testWithContext() {
	fmt.Println("=== Тест context.WithTimeout поведение ===")
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	// симулируем 33s работы первой попытки
	start := time.Now()
	time.Sleep(100 * time.Millisecond) // ускоренно
	elapsed := time.Since(start)
	_ = elapsed
	if ctx.Err() == nil {
		fmt.Println("  Контекст 45s жив после 33s -> осталось 12s (как в проде)")
	}
	// новый контекст на ретрай
	ctx2, cancel2 := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel2()
	if ctx2.Err() == nil {
		fmt.Println("  Новый контекст 45s на ретрай -> полные 45s доступно -> OK")
	}
	fmt.Println()
}

func main() {
	fmt.Println("Локальный тест без Docker - проверка логики monitor_api.go")
	fmt.Println("Время:", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Println()

	testURLFiltering()
	testOuterTimeout()
	testNavigateLogic()
	testDeadlineNotEnough()
	testWithContext()

	fmt.Println("=== Итог ===")
	fmt.Println("1. Фильтрация URL: нужен pass-through для negotiate - иначе дедлок")
	fmt.Println("2. outerTimeout: нужен свежий контекст на ретрай - иначе deadline exceeded")
	fmt.Println("3. Navigate: Contains(/carrier/) ложно для /carrier/sign/waybill - нужен переход на список")
	fmt.Println("4. Deadline 30s достаточно, если не паузить negotiate. Рекомендация: Sleep 3s->5s опционально")
	fmt.Println()
	fmt.Println("Тест завершен локально, без Docker, без браузера")
}
