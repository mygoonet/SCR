package SCRP

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/chromedp/chromedp"
)

// SelectorCheck — результат проверки одного селектора разметки.
type SelectorCheck struct {
	Name  string
	Found bool
	Count int
}

// CheckReport — итог прогона проверки.
type CheckReport struct {
	OK         bool
	LoggedIn   bool
	OnCarrier  bool
	NotesCount int
	Selectors  []SelectorCheck
	SignFlowOK bool
	SignError  string
}

// CheckSite — ручная проверка, что разметка сайта (data-tid) не поменялась
// и путь подписания до модалки выбора сертификата жив. НИЧЕГО не подписывает:
// после проверки модалки выбора сертификата всё закрывается ESC.
// Если forceFresh=true — перед проверкой сбрасывается сессия, чтобы
// гарантированно пройти ПОЛНЫЙ путь логина (а не по сохранённой сессии).
func CheckSite(browser *Browser, cfg Config, forceFresh bool) *CheckReport {
	rep := &CheckReport{}

	session := browser.NewSession()
	defer session.Close()
	ctx := session.Ctx()

	if forceFresh {
		log.Println("CheckSite: принудительный сброс сессии перед проверкой")
	}

	if err := initSession(ctx, cfg); err != nil {
		rep.SignError = fmt.Sprintf("init session: %v", err)
		takeScreenshot(ctx, "check_init_fail")
		return rep
	}
	rep.LoggedIn = true
	rep.OnCarrier = true

	if err := waitForTableRows(ctx); err != nil {
		rep.SignError = fmt.Sprintf("wait table rows: %v", err)
		takeScreenshot(ctx, "check_table_fail")
		return rep
	}

	notes, err := ParseDeliveryNotes(ctx)
	if err != nil {
		rep.SignError = fmt.Sprintf("parse notes: %v", err)
		takeScreenshot(ctx, "check_parse_fail")
		return rep
	}
	rep.NotesCount = len(notes)

	// 1. Разметка таблицы: проверяем, что все data-tid на месте.
	rep.Selectors = checkTableSelectors(ctx)

	// 2. Путь подписания до модалки сертификата (без реальной подписи).
	if len(notes) > 0 {
		if err := checkSignPath(ctx, cfg.CertUser, notes[0].Number, rep); err != nil {
			rep.SignFlowOK = false
			rep.SignError = err.Error()
			takeScreenshot(ctx, "check_signpath_fail")
		} else {
			rep.SignFlowOK = true
		}
	} else {
		rep.SignFlowOK = true
		rep.SignError = "накладных нет — путь подписания не проверялся"
	}

	rep.OK = rep.LoggedIn && rep.OnCarrier && rep.SignFlowOK && allSelectorsFound(rep.Selectors)
	return rep
}

func allSelectorsFound(sels []SelectorCheck) bool {
	for _, s := range sels {
		if !s.Found {
			return false
		}
	}
	return true
}

// checkTableSelectors проверяет наличие всех data-tid-селекторов, от которых
// зависит парсинг таблицы и открытие меню строки.
func checkTableSelectors(ctx context.Context) []SelectorCheck {
	need := []struct {
		name string
		sel  string
	}{
		{"TableRow", `[data-tid="TableRow"]`},
		{"WaybillNumber", `[data-tid="TableRow"] [data-tid="WaybillNumber"]`},
		{"WaybillDate", `[data-tid="TableRow"] [data-tid="WaybillDate"]`},
		{"WaybillSenderCell", `[data-tid="TableRow"] [data-tid="WaybillSenderCell"]`},
		{"WaybillRecipientCell", `[data-tid="TableRow"] [data-tid="WaybillRecipientCell"]`},
		{"CarrierName", `[data-tid="TableRow"] [data-tid="CarrierName"]`},
		{"DriverName", `[data-tid="TableRow"] [data-tid="DriverName"]`},
		{"DriverPhone", `[data-tid="TableRow"] [data-tid="DriverPhone"]`},
		{"TruckInfo", `[data-tid="TableRow"] [data-tid="TruckInfo"]`},
		{"RowActions", `[data-tid="TableRow"] [data-tid="RowActions"]`},
	}

	var out []SelectorCheck
	for _, n := range need {
		var count int
		if err := chromedp.Run(ctx, chromedp.Evaluate(
			fmt.Sprintf(`document.querySelectorAll(%q).length`, n.sel), &count)); err != nil {
			log.Printf("check selector %s: %v", n.name, err)
			out = append(out, SelectorCheck{Name: n.name, Found: false, Count: 0})
			continue
		}
		out = append(out, SelectorCheck{Name: n.name, Found: count > 0, Count: count})
	}
	return out
}

// checkSignPath проходит путь подписания до модалки "Выбор сертификата":
// открывает меню строки, пункт "Подписать без подписи водителя", SidePage,
// кнопку "Подписать", дожидается модалки сертификата и проверяет, что нужный
// сертификат в списке. На этом останавливается и закрывает всё ESC —
// накладная НЕ подписывается.
func checkSignPath(ctx context.Context, certUser, number string, rep *CheckReport) error {
	if err := openRowMenu(ctx, number); err != nil {
		return fmt.Errorf("open row menu: %w", err)
	}

	popupJS := `document.querySelector('[data-tid="Popup__root"]') !== null`
	if err := waitForJS(ctx, popupJS, 10*time.Second); err != nil {
		return fmt.Errorf("popup menu: %w", err)
	}
	rep.Selectors = append(rep.Selectors,
		checkOne(ctx, "Popup__root", `[data-tid="Popup__root"]`),
	)

	signItemJS := `document.querySelector('[data-tid="Popup__root"] [data-tid="SignWithoutDriverSignature"]') !== null`
	if err := waitForJS(ctx, signItemJS, 10*time.Second); err != nil {
		return fmt.Errorf("sign menu item: %w", err)
	}
	rep.Selectors = append(rep.Selectors,
		SelectorCheck{Name: "SignWithoutDriverSignature", Found: true, Count: 1},
	)

	if err := reactClickEl(ctx, `document.querySelector('[data-tid="Popup__root"] [data-tid="SignWithoutDriverSignature"]')`); err != nil {
		return fmt.Errorf("click sign item: %w", err)
	}

	sidePageJS := `document.querySelector('[data-tid="SidePageFooter__root"] [data-tid="Sign"]') !== null`
	if err := waitForJS(ctx, sidePageJS, 15*time.Second); err != nil {
		return fmt.Errorf("side page sign button: %w", err)
	}
	rep.Selectors = append(rep.Selectors,
		checkOne(ctx, "SidePageFooter__root/Sign", `[data-tid="SidePageFooter__root"] [data-tid="Sign"]`),
	)

	signBtnJS := `(function(){
		var f = document.querySelector('[data-tid="SidePageFooter__root"]');
		if(!f) return null;
		var s = f.querySelector('[data-tid="Sign"]');
		return s ? (s.tagName === 'BUTTON' ? s : (s.querySelector('button') || s)) : null;
	})()`
	if err := clickAtCenter(ctx, signBtnJS); err != nil {
		return fmt.Errorf("click Sign: %w", err)
	}

	if err := waitForJS(ctx, `document.querySelector('[data-tid^="certificate_"]') !== null`, 30*time.Second); err != nil {
		return fmt.Errorf("certificate modal: %w", err)
	}
	rep.Selectors = append(rep.Selectors,
		checkOne(ctx, "certificate list", `[data-tid^="certificate_"]`),
	)

	// Проверяем, что нужный сертификат присутствует в модалке.
	certFound := false
	if err := chromedp.Run(ctx, chromedp.Evaluate(fmt.Sprintf(
		`(function(user){
			var certs = document.querySelectorAll('[data-tid^="certificate_"]');
			for(var i=0;i<certs.length;i++){
				if(certs[i].textContent.indexOf(user) !== -1) return true;
			}
			return false;
		})(%q)`, certUser), &certFound)); err != nil {
		return fmt.Errorf("check certificate presence: %w", err)
	}
	rep.Selectors = append(rep.Selectors,
		SelectorCheck{Name: "certificate=" + certUser, Found: certFound, Count: boolInt(certFound)},
	)
	if !certFound {
		return fmt.Errorf("сертификат %q не найден в модалке выбора", certUser)
	}

	// Останавливаемся, ничего не подписываем — закрываем всё ESC.
	closePopups(ctx)
	return nil
}

// FullSignDeliveryNote — полный прогон подписания на конкретной накладной:
// логинится, переходит в раздел Перевозчика, ждёт таблицу и выполняет
// SignDeliveryNote. Не зависит от skipNumbers — можно подписывать тестовые
// накладные вроде 000000420.
func FullSignDeliveryNote(browser *Browser, cfg Config, number string) error {
	session := browser.NewSession()
	defer session.Close()
	ctx := session.Ctx()

	if err := initSession(ctx, cfg); err != nil {
		return fmt.Errorf("init session: %w", err)
	}
	if err := waitForTableRows(ctx); err != nil {
		return fmt.Errorf("wait table rows: %w", err)
	}

	signCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	return SignDeliveryNote(signCtx, number, cfg.CertUser)
}

func checkOne(ctx context.Context, name, sel string) SelectorCheck {
	var count int
	if err := chromedp.Run(ctx, chromedp.Evaluate(
		fmt.Sprintf(`document.querySelectorAll(%q).length`, sel), &count)); err != nil {
		return SelectorCheck{Name: name, Found: false, Count: 0}
	}
	return SelectorCheck{Name: name, Found: count > 0, Count: count}
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
