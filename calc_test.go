package main

import "testing"

func TestBreakEvenKnownCase(t *testing.T) {
	// Выручка 100 000, маржа 40%, переменные 10% -> коэф. маржин. прибыли 30%.
	// Постоянные 15 000 -> ТБУ = 15 000 / 0,30 = 50 000; прибыль = 30 000 - 15 000 = 15 000.
	in := Input{
		Revenue:       100000,
		PricingMode:   "margin",
		MarginPercent: 40,
		AvgCheck:      50,
		WorkingDays:   25,
		VariableCosts: []VariableCost{{Name: "Эквайринг и пр.", Percent: 10}},
		FixedCosts:    []FixedCost{{Name: "Аренда", Amount: 15000, BenchmarkPercent: 10}},
	}
	r := Compute(in)
	if diff := r.BreakEven - 50000; diff > 0.01 || diff < -0.01 {
		t.Fatalf("BreakEven = %v, ожидалось 50000", r.BreakEven)
	}
	if diff := r.Profit - 15000; diff > 0.01 || diff < -0.01 {
		t.Fatalf("Profit = %v, ожидалось 15000", r.Profit)
	}
	if diff := r.SafetyPercent - 50; diff > 0.01 || diff < -0.01 {
		t.Fatalf("SafetyPercent = %v, ожидалось 50", r.SafetyPercent)
	}
	// ТБУ в день = 2000, чеков в день = 2000/50 = 40.
	if diff := r.ChecksPerDayBE - 40; diff > 0.01 || diff < -0.01 {
		t.Fatalf("ChecksPerDayBE = %v, ожидалось 40", r.ChecksPerDayBE)
	}
}

func TestMarkupToMargin(t *testing.T) {
	// Наценка 100% -> маржа 50%.
	r := Compute(Input{Revenue: 1000, PricingMode: "markup", MarkupPercent: 100, WorkingDays: 30})
	if diff := r.GrossMarginPercent - 50; diff > 0.001 || diff < -0.001 {
		t.Fatalf("GrossMarginPercent = %v, ожидалось 50", r.GrossMarginPercent)
	}
}

func TestOptimizationScenario(t *testing.T) {
	// Снижение единственной статьи на 20%: 10 000 -> 8 000, ТБУ падает.
	in := Input{
		Revenue:       100000,
		PricingMode:   "margin",
		MarginPercent: 50,
		WorkingDays:   30,
		FixedCosts:    []FixedCost{{Name: "Аренда", Amount: 10000, OptimizePercent: 20}},
	}
	r := Compute(in)
	if r.Optimized.Saving != 2000 {
		t.Fatalf("Saving = %v, ожидалось 2000", r.Optimized.Saving)
	}
	if !(r.Optimized.BreakEven < r.BreakEven && r.Optimized.Profit > r.Profit) {
		t.Fatalf("оптимизация должна снижать ТБУ и растит прибыль: %+v", r.Optimized)
	}
}

func TestPotentialSavingIgnoresNoBenchmark(t *testing.T) {
	// Статья без норматива (BenchmarkPercent=0) не должна попадать
	// в PotentialSaving и в ExcessAmount (регрессионный тест к ревью 2026-10-19).
	r := Compute(Input{Revenue: 100000, PricingMode: "margin", MarginPercent: 50,
		WorkingDays: 30, FixedCosts: []FixedCost{{Name: "Прочее", Amount: 10000}}})
	if r.PotentialSaving != 0 {
		t.Fatalf("PotentialSaving = %v, ожидалось 0 без норматива", r.PotentialSaving)
	}
	if len(r.FixedAnalysis) == 1 && r.FixedAnalysis[0].ExcessAmount != 0 {
		t.Fatalf("ExcessAmount = %v, ожидалось 0 без норматива", r.FixedAnalysis[0].ExcessAmount)
	}
}

func TestZeroContributionNoPanic(t *testing.T) {
	r := Compute(Input{Revenue: 100, PricingMode: "margin", MarginPercent: 5,
		VariableCosts: []VariableCost{{Percent: 10}}, WorkingDays: 30})
	if r.BreakEven != 0 || len(r.Warnings) == 0 {
		t.Fatalf("при отрицательной марже ожидалось предупреждение и ТБУ=0, получено %+v", r)
	}
}
