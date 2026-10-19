// Анализ табличных отчётов вида «ОТЧЕТ ПО ОРГАНИЗАЦИИ …»:
// периоды по столбцам (кварталы/месяцы), показатели по строкам.
// Анализатор не привязан к конкретному файлу: заголовок периодов,
// колонка названий и ключевые строки (выручка, себестоимость, затраты,
// налоги) определяются эвристиками по тексту ячеек.
package main

import (
	"math"
	"sort"
	"strings"
)

// RowAnalysis — одна строка-показатель отчёта.
type RowAnalysis struct {
	Label          string    `json:"label"`
	Kind           string    `json:"kind"` // revenue|cogs|costsTotal|tax|vat|expense|other
	Values         []float64 `json:"values"`
	HasValue       []bool    `json:"hasValue"`
	Total          float64   `json:"total"`          // итого (колонка «Итого» или сумма периодов)
	Average        float64   `json:"average"`        // среднее за период
	ShareOfRevenue float64   `json:"shareOfRevenue"` // % от выручки (по Total)
	ShareOfCosts   float64   `json:"shareOfCosts"`   // % от суммы затрат (по Total)
	TrendPercent   float64   `json:"trendPercent"`   // последний период к первому, %
}

// SheetAnalysis — анализ одного листа.
type SheetAnalysis struct {
	Name         string        `json:"name"`
	Organization string        `json:"organization,omitempty"`
	Periods      []string      `json:"periods"`
	Rows         []RowAnalysis `json:"rows"`
	TopExpenses  []RowAnalysis `json:"topExpenses"`

	// Ключевые показатели (по итогам за все периоды).
	RevenueTotal    float64 `json:"revenueTotal"`
	COGSTotal       float64 `json:"cogsTotal"`
	CostsTotal      float64 `json:"costsTotal"`
	TaxTotal        float64 `json:"taxTotal"`
	GrossProfit     float64 `json:"grossProfit"`     // выручка − себестоимость
	GrossMarginPct  float64 `json:"grossMarginPct"`  // валовая маржа, %
	CostsSharePct   float64 `json:"costsSharePct"`   // затраты, % выручки
	ProfitBeforeTax float64 `json:"profitBeforeTax"` // выручка − себест. − затраты
	ProfitAfterTax  float64 `json:"profitAfterTax"`  // минус налоги (если найдены строки налогов)

	MonthsCovered  float64 `json:"monthsCovered"` // оценку длительности периода для калькулятора
	MonthlyRevenue float64 `json:"monthlyRevenue"`
	CalcSuggestion *Input  `json:"calcSuggestion,omitempty"` // готовые данные для калькулятора ТБУ

	Recommendations []string `json:"recommendations"`
	Warnings        []string `json:"warnings"`
}

// ReportAnalysis — результат анализа всего файла.
type ReportAnalysis struct {
	FileName string          `json:"fileName"`
	Format   string          `json:"format"`
	Sheets   []SheetAnalysis `json:"sheets"`
}

func classifyLabel(label string) string {
	l := strings.ToLower(strings.TrimSpace(label))
	switch {
	case strings.Contains(l, "налог"):
		return "tax"
	case strings.Contains(l, "итого выручка") || l == "выручка" || strings.HasPrefix(l, "выручка"):
		return "revenue"
	case strings.Contains(l, "себестоимость реализац") || strings.Contains(l, "себест") && strings.Contains(l, "реализ"):
		return "cogs"
	case strings.Contains(l, "себестоимость затрат") || strings.Contains(l, "итого затрат") || strings.Contains(l, "итого расходы"):
		return "costsTotal"
	case strings.Contains(l, "себест"):
		return "cogs"
	case strings.Contains(l, "ндс"):
		return "vat"
	case strings.Contains(l, "итого"):
		return "other"
	default:
		return "expense"
	}
}

func isPeriodHeader(s string) bool {
	l := strings.ToLower(strings.TrimSpace(s))
	if l == "" {
		return false
	}
	if strings.Contains(l, "кв") || strings.Contains(l, "мес") || strings.Contains(l, "итого") ||
		strings.Contains(l, "год") || strings.Contains(l, "период") ||
		strings.Contains(l, "q1") || strings.Contains(l, "q2") ||
		strings.Contains(l, "q3") || strings.Contains(l, "q4") {
		return true
	}
	// Месяцы: полное название или сокращение с точкой/как отдельное слово,
	// иначе «МИРСТРОЙМАРКЕТ» ложно матчится по «мар» (был такой баг).
	months := [][2]string{
		{"январь", "янв"}, {"февраль", "фев"}, {"март", "мар"}, {"апрель", "апр"},
		{"май", "май"}, {"июнь", "июн"}, {"июль", "июл"}, {"август", "авг"},
		{"сентябрь", "сен"}, {"октябрь", "окт"}, {"ноябрь", "ноя"}, {"декабрь", "дек"},
	}
	for _, m := range months {
		if l == m[0] || l == m[1] || strings.HasPrefix(l, m[0]+" ") ||
			strings.HasPrefix(l, m[1]+".") || strings.HasPrefix(l, m[1]+" ") {
			return true
		}
	}
	return false
}

func monthsPerPeriod(periods []string) float64 {
	for _, p := range periods {
		l := strings.ToLower(p)
		if strings.Contains(l, "кв") {
			return 3
		}
		if strings.Contains(l, "год") {
			return 12
		}
	}
	return 1
}

// AnalyzeWorkbook строит анализ по всем листам разобранной книги.
func AnalyzeWorkbook(wb *Workbook) ReportAnalysis {
	rep := ReportAnalysis{FileName: wb.FileName, Format: wb.Format}
	for _, sh := range wb.Sheets {
		if sa, ok := analyzeSheet(sh); ok {
			rep.Sheets = append(rep.Sheets, sa)
		}
	}
	return rep
}

func analyzeSheet(sh Sheet) (SheetAnalysis, bool) {
	// 1) Ищем строку заголовков периодов: >=2 периодных подписей.
	headerRow, firstCol, lastCol := -1, -1, -1
	for r := 0; r < len(sh.Rows) && r < 12; r++ {
		row := sh.Rows[r]
		count, first, last := 0, -1, -1
		for c, cell := range row {
			if isPeriodHeader(cell) {
				count++
				if first < 0 {
					first = c
				}
				last = c
			}
		}
		if count >= 2 {
			headerRow, firstCol, lastCol = r, first, last
			break
		}
	}
	if headerRow < 0 {
		return SheetAnalysis{}, false
	}
	var periods []string
	totalColOffset := -1 // индекс внутри periods колонки «Итого», если есть
	for c := firstCol; c <= lastCol; c++ {
		name := strings.TrimSpace(sh.Rows[headerRow][c])
		if name == "" {
			name = "Период"
		}
		if strings.Contains(strings.ToLower(name), "итого") {
			totalColOffset = len(periods)
		}
		periods = append(periods, name)
	}
	// 2) Колонка названий: столбец слева от периодов с максимумом текста ниже заголовка.
	labelCol := 0
	best := -1
	for c := 0; c < firstCol; c++ {
		cnt := 0
		for r := headerRow + 1; r < len(sh.Rows); r++ {
			if c < len(sh.Rows[r]) {
				if _, isNum := parseNumberCell(sh.Rows[r][c]); !isNum && strings.TrimSpace(sh.Rows[r][c]) != "" {
					cnt++
				}
			}
		}
		if cnt > best {
			best, labelCol = cnt, c
		}
	}
	// Организация: первый непустой текст выше/в строке заголовка вне периодов.
	org := ""
	for r := 0; r <= headerRow && org == ""; r++ {
		for c := 0; c < len(sh.Rows[r]); c++ {
			v := strings.TrimSpace(sh.Rows[r][c])
			if v != "" && !isPeriodHeader(v) {
				if _, isNum := parseNumberCell(v); !isNum && len(v) > 2 {
					org = v
					break
				}
			}
		}
	}

	sa := SheetAnalysis{Name: sh.Name, Organization: org, Periods: periods}
	// 3) Строки-показатели.
	for r := headerRow + 1; r < len(sh.Rows); r++ {
		row := sh.Rows[r]
		if labelCol >= len(row) {
			continue
		}
		label := strings.TrimSpace(row[labelCol])
		if label == "" {
			continue
		}
		ra := RowAnalysis{Label: label, Kind: classifyLabel(label)}
		any := false
		sum := 0.0
		for pc := 0; pc < len(periods); pc++ {
			c := firstCol + pc
			var v float64
			ok := false
			if c < len(row) {
				v, ok = parseNumberCell(row[c])
			}
			ra.Values = append(ra.Values, v)
			ra.HasValue = append(ra.HasValue, ok)
			if ok && pc != totalColOffset {
				sum += v
				any = true
			}
		}
		if !any && totalColOffset < 0 {
			continue
		}
		if totalColOffset >= 0 && ra.HasValue[totalColOffset] {
			ra.Total = ra.Values[totalColOffset]
		} else {
			ra.Total = sum
		}
		// Среднее и тренд — по периодам без «Итого».
		var cnt float64
		var firstV, lastV float64
		seen := false
		for pc := 0; pc < len(periods); pc++ {
			if pc == totalColOffset || !ra.HasValue[pc] {
				continue
			}
			cnt++
			if !seen {
				firstV, seen = ra.Values[pc], true
			}
			lastV = ra.Values[pc]
		}
		if cnt > 0 {
			ra.Average = ra.Total / cnt
			if totalColOffset < 0 {
				ra.Average = sum / cnt
			}
		}
		if cnt >= 2 && firstV != 0 {
			ra.TrendPercent = (lastV - firstV) / math.Abs(firstV) * 100
		}
		sa.Rows = append(sa.Rows, ra)
	}
	if len(sa.Rows) == 0 {
		return SheetAnalysis{}, false
	}

	// 4) Ключевые показатели.
	var revRow, cogsRow, costsRow *RowAnalysis
	for i := range sa.Rows {
		r := &sa.Rows[i]
		switch r.Kind {
		case "revenue":
			if revRow == nil || r.Total > revRow.Total {
				revRow = r
			}
		case "cogs":
			if cogsRow == nil {
				cogsRow = r
			}
		case "costsTotal":
			costsRow = r
		case "tax":
			sa.TaxTotal += r.Total
		}
	}
	if revRow != nil {
		sa.RevenueTotal = revRow.Total
	}
	if cogsRow != nil {
		sa.COGSTotal = cogsRow.Total
	}
	if costsRow != nil {
		sa.CostsTotal = costsRow.Total
	} else {
		for _, r := range sa.Rows {
			if r.Kind == "expense" {
				sa.CostsTotal += r.Total
			}
		}
	}
	for i := range sa.Rows {
		if sa.RevenueTotal != 0 {
			sa.Rows[i].ShareOfRevenue = sa.Rows[i].Total / sa.RevenueTotal * 100
		}
		if sa.CostsTotal != 0 {
			sa.Rows[i].ShareOfCosts = sa.Rows[i].Total / sa.CostsTotal * 100
		}
	}
	sa.GrossProfit = sa.RevenueTotal - sa.COGSTotal
	if sa.RevenueTotal != 0 {
		sa.GrossMarginPct = sa.GrossProfit / sa.RevenueTotal * 100
		sa.CostsSharePct = sa.CostsTotal / sa.RevenueTotal * 100
	}
	sa.ProfitBeforeTax = sa.GrossProfit - sa.CostsTotal
	sa.ProfitAfterTax = sa.ProfitBeforeTax - sa.TaxTotal

	// Топ накладных (детальные статьи затрат).
	for _, r := range sa.Rows {
		if r.Kind == "expense" && r.Total != 0 {
			sa.TopExpenses = append(sa.TopExpenses, r)
		}
	}
	sort.Slice(sa.TopExpenses, func(i, j int) bool { return math.Abs(sa.TopExpenses[i].Total) > math.Abs(sa.TopExpenses[j].Total) })
	if len(sa.TopExpenses) > 10 {
		sa.TopExpenses = sa.TopExpenses[:10]
	}

	// Длительность: периоды без «Итого» × месяцев в периоде.
	nPeriods := 0
	for i := range periods {
		if i != totalColOffset {
			nPeriods++
		}
	}
	if nPeriods == 0 {
		nPeriods = len(periods)
	}
	sa.MonthsCovered = float64(nPeriods) * monthsPerPeriod(periods)
	if sa.MonthsCovered > 0 {
		sa.MonthlyRevenue = sa.RevenueTotal / sa.MonthsCovered
	}

	// 5) Подсказка для калькулятора ТБУ: средние за месяц.
	if sa.MonthsCovered > 0 && sa.RevenueTotal > 0 {
		in := &Input{
			Revenue:       round2(sa.MonthlyRevenue),
			PricingMode:   "margin",
			MarginPercent: round2(sa.GrossMarginPct),
			WorkingDays:   30,
		}
		for _, r := range sa.Rows {
			if r.Kind == "expense" && r.Total > 0 {
				in.FixedCosts = append(in.FixedCosts, FixedCost{
					Name:   r.Label,
					Amount: round2(r.Total / sa.MonthsCovered),
				})
			}
		}
		sa.CalcSuggestion = in
	}

	// 6) Выводы и предупреждения.
	for i, p := range periods {
		empty := true
		for _, r := range sa.Rows {
			if i < len(r.HasValue) && r.HasValue[i] && r.Values[i] != 0 {
				empty = false
			}
		}
		if empty && i != totalColOffset {
			sa.Warnings = append(sa.Warnings, "Период «"+p+"» пуст или равен нулю — итоги занижены, динамика считается по заполненным периодам.")
		}
	}
	if revRow == nil {
		sa.Warnings = append(sa.Warnings, "Строка выручки не найдена (ищем «Итого выручка»/«Выручка») — доли и маржа не посчитаны.")
	}
	if sa.GrossMarginPct > 0 && sa.GrossMarginPct < 15 && revRow != nil {
		sa.Recommendations = append(sa.Recommendations, "Валовая маржа ниже 15% — точка безубыточности очень чувствительна к накладным; в приоритете наценка и закупочные цены.")
	}
	if len(sa.TopExpenses) > 0 && sa.TopExpenses[0].ShareOfCosts >= 40 {
		top := sa.TopExpenses[0]
		sa.Recommendations = append(sa.Recommendations, "Крупнейшая статья «"+top.Label+"» — это основная доля затрат; её снижение даёт максимальный сдвиг точки безубыточности. Меры: "+tipFor(top.Label)+".")
	}
	if revRow != nil && math.Abs(revRow.TrendPercent) >= 5 {
		dir := "выросла"
		if revRow.TrendPercent < 0 {
			dir = "снизилась"
		}
		sa.Recommendations = append(sa.Recommendations, "Выручка за последний заполненный период "+dir+" относительно первого; проверьте, повторяется ли тренд, и заложите его в план выручки калькулятора.")
	}
	if sa.CostsSharePct >= 25 {
		sa.Recommendations = append(sa.Recommendations, "Накладные затраты занимают заметную долю выручки — используйте блок оптимизации калькулятора: статьи уже можно перенести кнопкой «В расчёт безубыточности».")
	}
	if len(sa.Recommendations) == 0 {
		sa.Recommendations = append(sa.Recommendations, "Явных аномалий не найдено: перенесите данные в калькулятор кнопкой ниже и проверьте запас прочности на сценарии оптимизации.")
	}
	return sa, true
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }
