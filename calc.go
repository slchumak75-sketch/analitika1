package main

import (
	"math"
	"sort"
	"strings"
)

// ---------- Входные данные ----------

// VariableCost — переменный расход, % от выручки (эквайринг, потери и т.п.).
type VariableCost struct {
	Name    string  `json:"name"`
	Percent float64 `json:"percent"`
}

// FixedCost — постоянный / накладной расход, руб. в месяц.
type FixedCost struct {
	Name             string  `json:"name"`
	Amount           float64 `json:"amount"`           // руб./мес
	BenchmarkPercent float64 `json:"benchmarkPercent"` // норматив, % от выручки
	OptimizePercent  float64 `json:"optimizePercent"`  // сценарий: снизить статью на, %
}

// Input — все параметры магазина, присылаемые из HTML-интерфейса.
type Input struct {
	Revenue       float64        `json:"revenue"`     // выручка за месяц
	PricingMode   string         `json:"pricingMode"` // "markup" — наценка, "margin" — маржа
	MarkupPercent float64        `json:"markupPercent"`
	MarginPercent float64        `json:"marginPercent"`
	AvgCheck      float64        `json:"avgCheck"`     // средний Чек
	WorkingDays   float64        `json:"workingDays"`  // рабочих дней в месяце
	TargetProfit  float64        `json:"targetProfit"` // целевая прибыль за месяц
	VariableCosts []VariableCost `json:"variableCosts"`
	FixedCosts    []FixedCost    `json:"fixedCosts"`
}

// ---------- Результаты ----------

type FixedAnalysis struct {
	Name             string  `json:"name"`
	Amount           float64 `json:"amount"`
	ShareOfRevenue   float64 `json:"shareOfRevenue"` // % от выручки
	ShareOfFixed     float64 `json:"shareOfFixed"`   // % в сумме накладных
	BenchmarkPercent float64 `json:"benchmarkPercent"`
	BenchmarkAmount  float64 `json:"benchmarkAmount"` // норматив в рублях
	ExcessAmount     float64 `json:"excessAmount"`    // превышение норматива, руб.
	Status           string  `json:"status"`          // ok | warn | over
	OptimizePercent  float64 `json:"optimizePercent"`
	OptimizedAmount  float64 `json:"optimizedAmount"`
	Saving           float64 `json:"saving"`
}

type Scenario struct {
	DeltaPercent    float64 `json:"deltaPercent"`
	Revenue         float64 `json:"revenue"`
	Profit          float64 `json:"profit"`
	ProfitOptimized float64 `json:"profitOptimized"`
}

type Point struct {
	X         float64 `json:"x"`
	Revenue   float64 `json:"revenue"`
	TotalCost float64 `json:"totalCost"`
}

type ScenarioResult struct {
	FixedTotal    float64 `json:"fixedTotal"`
	Saving        float64 `json:"saving"`
	BreakEven     float64 `json:"breakEven"`
	Profit        float64 `json:"profit"`
	SafetyPercent float64 `json:"safetyPercent"`
}

type Result struct {
	// Маржа и маржинальная прибыль
	GrossMarginPercent  float64 `json:"grossMarginPercent"`  // Валовая маржа, %
	COGS                float64 `json:"cogs"`                // себестоимость товара
	COGSPercent         float64 `json:"cogsPercent"`         // себестоимость, % выручки
	VariablePercent     float64 `json:"variablePercent"`     // сумма переменных, %
	VariableTotal       float64 `json:"variableTotal"`       // сумма переменных, руб.
	ContributionPercent float64 `json:"contributionPercent"` // маржинальность после переменных, %
	Contribution        float64 `json:"contribution"`        // маржинальная прибыль, руб.

	// Постоянные и прибыль
	FixedTotal        float64 `json:"fixedTotal"`
	Profit            float64 `json:"profit"`
	Profitability     float64 `json:"profitability"` // рентабельность продаж, %
	OperatingLeverage float64 `json:"operatingLeverage"`

	// Точка безубыточности
	BreakEven          float64 `json:"breakEven"`          // руб./мес
	BreakEvenPerDay    float64 `json:"breakEvenPerDay"`    // руб./день
	ChecksPerDayBE     float64 `json:"checksPerDayBE"`     // чеков в день для ТБУ
	ChecksPerDayNow    float64 `json:"checksPerDayNow"`    // чеков в день сейчас
	SafetyPercent      float64 `json:"safetyPercent"`      // запас финансовой прочности, %
	SafetyAmount       float64 `json:"safetyAmount"`       // запас прочности, руб.
	RevenueForTarget   float64 `json:"revenueForTarget"`   // выручка для целевой прибыли
	ChecksForTarget    float64 `json:"checksForTarget"`    // чеков в день для целевой прибыли
	ExtraRevenueNeeded float64 `json:"extraRevenueNeeded"` // сколько не хватает до ТБУ (0, если выше)

	// Оптимизация накладных
	FixedAnalysis     []FixedAnalysis `json:"fixedAnalysis"`
	Optimized         ScenarioResult  `json:"optimized"`         // сценарий ползунков «снизить на %»
	BenchmarkScenario ScenarioResult  `json:"benchmarkScenario"` // сценарий «все статьи — по нормативу»
	PotentialSaving   float64         `json:"potentialSaving"`   // сумма превышений нормативов

	Scenarios       []Scenario `json:"scenarios"`
	Chart           []Point    `json:"chart"`
	Recommendations []string   `json:"recommendations"`
	Warnings        []string   `json:"warnings"`
}

func clamp(v, lo, hi float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return lo
	}
	return math.Max(lo, math.Min(hi, v))
}

func safeDiv(a, b float64) float64 {
	if b == 0 {
		return 0
	}
	return a / b
}

// Compute — главная расчётная функция.
func Compute(in Input) Result {
	revenue := clamp(in.Revenue, 0, 1e15)
	days := clamp(in.WorkingDays, 1, 31)
	if in.WorkingDays == 0 {
		days = 30
	}
	avgCheck := clamp(in.AvgCheck, 0, 1e12)
	target := clamp(in.TargetProfit, 0, 1e15)

	// Валовая маржа: из наценки или напрямую.
	var marginRatio float64
	switch in.PricingMode {
	case "margin":
		marginRatio = clamp(in.MarginPercent, -100, 99.9) / 100
	default: // markup: маржа = наценка / (1 + наценка)
		mk := clamp(in.MarkupPercent, -99, 10000) / 100
		marginRatio = safeDiv(mk, 1+mk)
	}

	var varRatio float64
	for _, v := range in.VariableCosts {
		varRatio += clamp(v.Percent, 0, 100) / 100
	}

	contributionRatio := marginRatio - varRatio

	var fixedTotal, optimizedFixed float64
	for _, f := range in.FixedCosts {
		amt := clamp(f.Amount, 0, 1e15)
		opt := clamp(f.OptimizePercent, 0, 100)
		fixedTotal += amt
		optimizedFixed += amt * (1 - opt/100)
	}

	res := Result{}
	res.GrossMarginPercent = marginRatio * 100
	res.COGSPercent = (1 - marginRatio) * 100
	res.COGS = revenue * (1 - marginRatio)
	res.VariablePercent = varRatio * 100
	res.VariableTotal = revenue * varRatio
	res.ContributionPercent = contributionRatio * 100
	res.Contribution = revenue * contributionRatio
	res.FixedTotal = fixedTotal
	res.Profit = res.Contribution - fixedTotal
	res.Profitability = safeDiv(res.Profit, revenue) * 100
	if math.Abs(res.Profit) > 0.005 {
		res.OperatingLeverage = safeDiv(res.Contribution, res.Profit)
	}

	// Точка безубыточности: ТБУ = Постоянные / Коэф. маржинальной прибыли.
	if contributionRatio > 0 {
		res.BreakEven = fixedTotal / contributionRatio
		res.RevenueForTarget = (fixedTotal + target) / contributionRatio
	}
	res.BreakEvenPerDay = safeDiv(res.BreakEven, days)
	res.ChecksPerDayBE = safeDiv(res.BreakEvenPerDay, avgCheck)
	res.ChecksPerDayNow = safeDiv(safeDiv(revenue, days), avgCheck)
	res.ChecksForTarget = safeDiv(safeDiv(res.RevenueForTarget, days), avgCheck)
	res.SafetyAmount = revenue - res.BreakEven
	res.SafetyPercent = safeDiv(res.SafetyAmount, revenue) * 100
	if res.SafetyAmount < 0 {
		res.ExtraRevenueNeeded = -res.SafetyAmount
	}

	// Анализ накладных расходов.
	res.FixedAnalysis = make([]FixedAnalysis, 0, len(in.FixedCosts))
	benchmarkFixed := fixedTotal
	for _, f := range in.FixedCosts {
		amt := clamp(f.Amount, 0, 1e15)
		benchPct := clamp(f.BenchmarkPercent, 0, 100)
		opt := clamp(f.OptimizePercent, 0, 100)
		benchAmt := revenue * benchPct / 100
		excess := math.Max(0, amt-benchAmt)
		if benchPct > 0 {
			benchmarkFixed -= excess
		}
		share := safeDiv(amt, revenue) * 100
		status := "ok"
		if benchPct > 0 {
			switch {
			case share > benchPct*1.2:
				status = "over"
			case share > benchPct:
				status = "warn"
			}
		}
		// ИСПРАВЛЕНО (ревью 2026-10-19): превышение считаем только при заданном
		// нормативе. Было: при benchPct==0 excess = вся сумма статьи, и
		// PotentialSaving завышался на все статьи без норматива (при этом
		// benchmarkFixed такие статьи корректно не трогал — логика расходилась).
		if benchPct <= 0 {
			excess = 0
		}
		res.FixedAnalysis = append(res.FixedAnalysis, FixedAnalysis{
			Name:             f.Name,
			Amount:           amt,
			ShareOfRevenue:   share,
			ShareOfFixed:     safeDiv(amt, fixedTotal) * 100,
			BenchmarkPercent: benchPct,
			BenchmarkAmount:  benchAmt,
			ExcessAmount:     excess,
			Status:           status,
			OptimizePercent:  opt,
			OptimizedAmount:  amt * (1 - opt/100),
			Saving:           amt * opt / 100,
		})
		res.PotentialSaving += excess
	}

	makeScenario := func(fixed float64) ScenarioResult {
		s := ScenarioResult{FixedTotal: fixed, Saving: fixedTotal - fixed}
		if contributionRatio > 0 {
			s.BreakEven = fixed / contributionRatio
		}
		s.Profit = res.Contribution - fixed
		s.SafetyPercent = safeDiv(revenue-s.BreakEven, revenue) * 100
		return s
	}
	res.Optimized = makeScenario(optimizedFixed)
	res.BenchmarkScenario = makeScenario(benchmarkFixed)

	// Сценарный анализ выручки ±20%.
	for _, d := range []float64{-20, -10, 0, 10, 20} {
		r := revenue * (1 + d/100)
		c := r * contributionRatio
		res.Scenarios = append(res.Scenarios, Scenario{
			DeltaPercent:    d,
			Revenue:         r,
			Profit:          c - fixedTotal,
			ProfitOptimized: c - optimizedFixed,
		})
	}

	// Данные графика безубыточности.
	maxX := math.Max(revenue, res.BreakEven) * 1.25
	if maxX <= 0 {
		maxX = 100
	}
	const steps = 24
	for i := 0; i <= steps; i++ {
		x := maxX * float64(i) / steps
		res.Chart = append(res.Chart, Point{
			X:         x,
			Revenue:   x,
			TotalCost: fixedTotal + x*(1-contributionRatio),
		})
	}

	res.Warnings, res.Recommendations = buildAdvice(in, res, contributionRatio)
	return res
}

func tipFor(name string) string {
	n := strings.ToLower(name)
	switch {
	case strings.Contains(n, "аренд"):
		return "переговоры о ставке или арендных каникулах, привязка платежа к % от оборота, сдача части площади в субаренду, сокращение неиспользуемой площади"
	case strings.Contains(n, "фот"), strings.Contains(n, "зарплат"), strings.Contains(n, "персонал"), strings.Contains(n, "оплат"):
		return "графики смен по часам пиковой нагрузки, совмещение функций, премиальная часть — в % от выручки/маржи вместо фиксированных окладов, норматив выручки на сотрудника"
	case strings.Contains(n, "коммун"), strings.Contains(n, "электр"), strings.Contains(n, "отопл"):
		return "светодиодное освещение и датчики, ревизия тарифов и договорных объёмов, утепление входной группы, контроль работы оборудования вне часов пик"
	case strings.Contains(n, "реклам"), strings.Contains(n, "маркетин"):
		return "оценка каждого канала по ROMI и стоимости привлечённого чека, отключение каналов без измеримой отдачи, сдвиг бюджета в программу лояльности и повторные продажи"
	case strings.Contains(n, "охра"), strings.Contains(n, "безопас"):
		return "тендер охранных услуг, замена части постов техническими средствами (камеры, противокражные системы) с оценкой влияния на потери"
	case strings.Contains(n, "бухг"), strings.Contains(n, "юрид"), strings.Contains(n, "аутсорс"):
		return "сравнение стоимости штатной функции и аутсорсинга, тендер подрядчика, сокращение объёма услуг до фактически нужных"
	case strings.Contains(n, "связ"), strings.Contains(n, "интернет"), strings.Contains(n, " по"), strings.Contains(n, "касс"):
		return "ревизия подписок и тарифов, отключение неиспользуемых сервисов и лицензий, объединение услуг у одного провайдера"
	case strings.Contains(n, "ремонт"), strings.Contains(n, "обслужив"), strings.Contains(n, "оборуд"):
		return "плановое обслуживание вместо аварийных ремонтов, тендер подрядчиков, оценка замены оборудования с хроническими поломками"
	case strings.Contains(n, "хоз"), strings.Contains(n, "расход"):
		return "нормы расхода на месяц, централизованная закупка, тендер поставщиков расходных материалов"
	case strings.Contains(n, "банк"):
		return "сравнение тарифных пакетов банков, переговоры о ставке эквайринга при растущем обороте"
	default:
		return "пересмотр договора и тендер поставщика, установление месячной нормы расхода и ответственного за статью"
	}
}

func buildAdvice(in Input, res Result, contributionRatio float64) (warnings, recs []string) {
	if contributionRatio <= 0 {
		warnings = append(warnings, "Маржинальная прибыль не положительная: Валовая маржа полностью «съедается» переменными расходами. При таких параметрах точка безубыточности недостижима — нужно повышать наценку/маржу или сокращать переменные расходы (% от выручки).")
		return warnings, recs
	}
	if in.Revenue <= 0 {
		warnings = append(warnings, "Не задана выручка — расчёт запаса прочности и нормативов невозможен.")
	}

	if res.Profit < 0 {
		warnings = append(warnings, "Магазин работает ниже точки безубыточности: текущий убыток формируется каждый месяц. Закрыть разрыв можно ростом выручки до точки безубыточности, сокращением постоянных расходов как минимум на сумму убытка или комбинацией обоих мер.")
	} else if res.SafetyPercent < 10 {
		warnings = append(warnings, "Запас финансовой прочности ниже 10% — даже небольшое сезонное падение выручки уведёт магазин в убыток. Приоритет: снижение постоянных расходов и рост маржинальности.")
	} else if res.SafetyPercent < 20 {
		warnings = append(warnings, "Запас прочности 10–20% — умеренный риск. Желательно довести его до 25–30% за счёт оптимизации крупнейших статей.")
	}

	// Статьи выше норматива — по убыванию суммы превышения.
	over := make([]FixedAnalysis, 0)
	for _, a := range res.FixedAnalysis {
		if a.ExcessAmount > 0 && a.BenchmarkPercent > 0 {
			over = append(over, a)
		}
	}
	sort.Slice(over, func(i, j int) bool { return over[i].ExcessAmount > over[j].ExcessAmount })
	for i, a := range over {
		if i >= 5 {
			break
		}
		recs = append(recs, "Статья «"+a.Name+"» выше норматива — резерв экономии есть; меры: "+tipFor(a.Name)+".")
	}

	// Крупнейшая статья вообще.
	if len(res.FixedAnalysis) > 0 {
		sorted := append([]FixedAnalysis(nil), res.FixedAnalysis...)
		sort.Slice(sorted, func(i, j int) bool { return sorted[i].Amount > sorted[j].Amount })
		top := sorted[0]
		if top.ShareOfFixed >= 25 {
			recs = append(recs, "Крупнейшая статья накладных — «"+top.Name+"»: даже её снижение на 5% даст заметный сдвиг точки безубыточности. Рассмотрите: "+tipFor(top.Name)+".")
		}
		// Принцип Парето.
		cum, cnt := 0.0, 0
		for _, a := range sorted {
			cum += a.ShareOfFixed
			cnt++
			if cum >= 80 {
				break
			}
		}
		if cnt > 0 && cnt < len(sorted) {
			recs = append(recs, "По принципу Парето основная часть накладных сосредоточена в нескольких первых статьях списка — оптимизацию начинайте с них, мелкие статьи почти не двигают точку безубыточности.")
		}
	}

	if res.Optimized.Saving > 0 {
		recs = append(recs, "Выбранный сценарий оптимизации снижает точку безубыточности и повышает запас прочности — конкретные цифры см. в блоке «Сценарий оптимизации».")
	}
	if res.Profit >= 0 && res.OperatingLeverage >= 5 {
		warnings = append(warnings, "Высокий операционный рычаг: прибыль очень чувствительна к выручке — падение продаж на 1% снижает прибыль кратно сильнее. Это следствие высокой доли постоянных расходов.")
	}
	return warnings, recs
}
