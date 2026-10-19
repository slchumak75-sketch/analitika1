package main

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

// Структура, повторяющая «ОТЧЕТ ПО ОРГАНИЗАЦИИ мир.xls» (сокращённо):
// организация, кварталы, выручка/себестоимость/затраты-детали/итого затрат.
func reportFixture() Sheet {
	return Sheet{Name: "Лист1", Rows: [][]string{
		{"", "МИРСТРОЙМАРКЕТ"},
		{"", "", "", "1кв", "2кв", "3кв", "4кв", "Итого"},
		{"", "Итого выручка", "", "258823.44", "586039.27", "623142.7", "0", "1468005.41"},
		{"", "Себестоимость реализация", "", "152812.9", "344951.64", "368269.77", "", "866034.31"},
		{"", "Себестоимость затрат", "", "61419.34", "86998.79", "96971.77", "0", "245389.9"},
		{"", "фонд з/пл", "0", "32789.98", "43329.24", "44803.28", "", "120922.5"},
		{"", "аренда", "0", "4037.76", "4037.76", "4037.76", "", "12113.28"},
		{"", "Налог на прибыль", "", "546.18", "11592.64", "11201.84", "0", "23340.66"},
	}}
}

func TestAnalyzeReportLikeMir(t *testing.T) {
	sa, ok := analyzeSheet(reportFixture())
	if !ok {
		t.Fatal("лист не проанализирован")
	}
	if sa.Organization != "МИРСТРОЙМАРКЕТ" {
		t.Fatalf("Organization = %q", sa.Organization)
	}
	if len(sa.Periods) != 5 {
		t.Fatalf("Periods = %v", sa.Periods)
	}
	if diff := sa.RevenueTotal - 1468005.41; diff > 0.01 || diff < -0.01 {
		t.Fatalf("RevenueTotal = %v", sa.RevenueTotal)
	}
	if diff := sa.GrossMarginPct - 41.0; diff > 0.2 || diff < -0.2 {
		t.Fatalf("GrossMarginPct = %v, ожидалось ~41", sa.GrossMarginPct)
	}
	if diff := sa.CostsTotal - 245389.9; diff > 0.01 || diff < -0.01 {
		t.Fatalf("CostsTotal = %v (должна браться строка итога затрат, а не сумма деталей)", sa.CostsTotal)
	}
	if len(sa.TopExpenses) == 0 || sa.TopExpenses[0].Label != "фонд з/пл" {
		t.Fatalf("TopExpenses = %+v", sa.TopExpenses)
	}
	if sa.MonthsCovered != 12 { // 4 квартала (вкл. пустой) x 3 мес — пустой период даёт warning
		t.Fatalf("MonthsCovered = %v", sa.MonthsCovered)
	}
	if len(sa.Warnings) == 0 {
		t.Fatal("ожидалось предупреждение о пустом 4кв")
	}
	if sa.CalcSuggestion == nil || len(sa.CalcSuggestion.FixedCosts) != 2 {
		t.Fatalf("CalcSuggestion = %+v", sa.CalcSuggestion)
	}
}

func TestParseCSVSemicolon1251(t *testing.T) {
	wb, err := ParseSpreadsheet("t.csv", []byte("Показатель;1кв;2кв\nВыручка;100;200\n"))
	if err != nil {
		t.Fatal(err)
	}
	rep := AnalyzeWorkbook(wb)
	if len(rep.Sheets) != 1 || rep.Sheets[0].RevenueTotal != 300 {
		t.Fatalf("rep = %+v", rep)
	}
}

func TestParseXLSXMinimal(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("xl/sharedStrings.xml")
	_, _ = w.Write([]byte(`<?xml version="1.0"?><sst><si><t>Показатель</t></si><si><t>1кв</t></si><si><t>Итого</t></si><si><t>Итого выручка</t></si></sst>`))
	w2, _ := zw.Create("xl/worksheets/sheet1.xml")
	_, _ = w2.Write([]byte(`<?xml version="1.0"?><worksheet><sheetData>` +
		`<row><c r="A1" t="s"><v>0</v></c><c r="B1" t="s"><v>1</v></c><c r="C1" t="s"><v>2</v></c></row>` +
		`<row><c r="A2" t="s"><v>3</v></c><c r="B2"><v>100</v></c><c r="C2"><v>100</v></c></row>` +
		`</sheetData></worksheet>`))
	_ = zw.Close()
	wb, err := ParseSpreadsheet("t.xlsx", buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	rep := AnalyzeWorkbook(wb)
	if len(rep.Sheets) != 1 || rep.Sheets[0].RevenueTotal != 100 {
		t.Fatalf("rep = %+v", rep)
	}
}

func TestDecodeRK(t *testing.T) {
	// RK целого числа 42: (42<<2)|2
	if v := decodeRK(uint32(42<<2 | 2)); v != 42 {
		t.Fatalf("decodeRK int = %v", v)
	}
	// RK с делением на 100: значение 1.5 -> хранится 150
	if v := decodeRK(uint32(150<<2 | 2 | 1)); v != 1.5 {
		t.Fatalf("decodeRK div100 = %v", v)
	}
}

func TestParseSpreadsheetRejectsUnknown(t *testing.T) {
	if _, err := ParseSpreadsheet("t.pdf", []byte("%PDF")); err == nil {
		t.Fatal("ожидалась ошибка формата")
	}
	if !strings.Contains("ok", "o") {
		t.Fatal("unreachable")
	}
}
