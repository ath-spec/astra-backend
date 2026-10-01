// Package rmbff holds the view-model types served by the RM console's BFF
// (/api/rm/bff/*). Field names and JSON tags mirror the TypeScript interfaces
// in astra-rm/src/types/index.ts exactly, so the frontend can consume the
// response without a mapping layer.
package rmbff

// --- Dashboard ---

type KpiFigure struct {
	Label      string   `json:"label"`
	Value      string   `json:"value"`
	DeltaPct   *float64 `json:"deltaPct,omitempty"`
	DeltaLabel string   `json:"deltaLabel,omitempty"`
	SubLabel   string   `json:"subLabel,omitempty"`
	Tone       string   `json:"tone"` // neutral | good | watch | risk | critical
}

type DashboardKpis struct {
	TotalExposure KpiFigure `json:"totalExposure"`
	Customers     KpiFigure `json:"customers"`
	ActiveLoans   KpiFigure `json:"activeLoans"`
	Healthy       KpiFigure `json:"healthy"`
	Watchlist     KpiFigure `json:"watchlist"`
	AtRisk        KpiFigure `json:"atRisk"`
	Critical      KpiFigure `json:"critical"`
}

type AiInsight struct {
	Severity string `json:"severity"` // critical | warning
	Text     string `json:"text"`
}

type AiPortfolioSummary struct {
	Badge    string      `json:"badge"`
	Headline string      `json:"headline"`
	Body     string      `json:"body"`
	Insights []AiInsight `json:"insights"`
	CtaLabel string      `json:"ctaLabel"`
}

type PortfolioHealthSegment struct {
	Label string  `json:"label"`
	Count int     `json:"count"`
	Pct   float64 `json:"pct"`
	Color string  `json:"color"`
}

type PortfolioHealth struct {
	TotalCustomers int                      `json:"totalCustomers"`
	Segments       []PortfolioHealthSegment `json:"segments"`
}

type LoanProductSlice struct {
	Label   string  `json:"label"`
	ValueCr float64 `json:"valueCr"`
	Pct     float64 `json:"pct"`
	Color   string  `json:"color"`
}

type LoanProductExposure struct {
	TotalCr  float64            `json:"totalCr"`
	Products []LoanProductSlice `json:"products"`
}

type RepaymentMonth struct {
	Label                   string  `json:"label"`
	OnTimePct               float64 `json:"onTimePct"`
	DelayedPct              float64 `json:"delayedPct"`
	CollectionEfficiencyPct float64 `json:"collectionEfficiencyPct"`
}

type RepaymentPerformance struct {
	RangeLabel           string           `json:"rangeLabel"`
	Months               []RepaymentMonth `json:"months"`
	CurrentEfficiencyPct float64          `json:"currentEfficiencyPct"`
	EfficiencyDeltaPct   float64          `json:"efficiencyDeltaPct"`
}

type RiskBand struct {
	Label string  `json:"label"`
	Count int     `json:"count"`
	Pct   float64 `json:"pct"`
	Tone  string  `json:"tone"` // excellent | good | fair | poor | high-risk
}

type PriorityCustomer struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Product          string `json:"product"`
	OutstandingLabel string `json:"outstandingLabel"`
	DaysPastDue      int    `json:"daysPastDue"`
	Risk             string `json:"risk"` // Critical | High | Watchlist
	KeyIssue         string `json:"keyIssue"`
}

type WorkItem struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Count int    `json:"count"`
	Icon  string `json:"icon"` // review | followup | document | application | crosssell
}

type DashboardData struct {
	GreetingName      string               `json:"greetingName"`
	BusinessUnit      string               `json:"businessUnit"`
	Quote             string               `json:"quote"`
	QuoteAttribution  string               `json:"quoteAttribution"`
	AsOfLabel         string               `json:"asOfLabel"`
	Kpis              DashboardKpis        `json:"kpis"`
	AiSummary         AiPortfolioSummary   `json:"aiSummary"`
	PortfolioHealth   PortfolioHealth      `json:"portfolioHealth"`
	LoanExposure      LoanProductExposure  `json:"loanExposure"`
	Repayment         RepaymentPerformance `json:"repayment"`
	RiskDistribution  []RiskBand           `json:"riskDistribution"`
	PriorityCustomers []PriorityCustomer   `json:"priorityCustomers"`
	WorkItems         []WorkItem           `json:"workItems"`
	WorkFocusQuote    string               `json:"workFocusQuote"`
}

// --- Portfolio ---

type PortfolioKpi struct {
	Label          string   `json:"label"`
	Value          string   `json:"value"`
	DeltaPct       *float64 `json:"deltaPct,omitempty"`
	DeltaDirection string   `json:"deltaDirection,omitempty"` // up | down
	DeltaLabel     string   `json:"deltaLabel,omitempty"`
	Tone           string   `json:"tone"` // good | info | watch | risk | critical
}

type PortfolioKpis struct {
	TotalCustomers PortfolioKpi `json:"totalCustomers"`
	TotalExposure  PortfolioKpi `json:"totalExposure"`
	ActiveLoans    PortfolioKpi `json:"activeLoans"`
	Watchlist      PortfolioKpi `json:"watchlist"`
	AtRisk         PortfolioKpi `json:"atRisk"`
	Critical       PortfolioKpi `json:"critical"`
}

type FilterOption struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Placeholder string `json:"placeholder"`
}

type PortfolioTab struct {
	Key   string `json:"key"` // all | watchlist | at-risk | critical
	Label string `json:"label"`
	Count int    `json:"count"`
}

// CashFlowTrend is {kind:"stable"} or {kind:"up"|"down", pct}.
type CashFlowTrend struct {
	Kind string   `json:"kind"`
	Pct  *float64 `json:"pct,omitempty"`
}

type CustomerRow struct {
	ID            string        `json:"id"`
	Name          string        `json:"name"`
	CustomerID    string        `json:"customerId"`
	Product       string        `json:"product"`
	Region        string        `json:"region"`
	ExposureLabel string        `json:"exposureLabel"`
	CreditScore   int           `json:"creditScore"`
	CashFlowTrend CashFlowTrend `json:"cashFlowTrend"`
	DPD           int           `json:"dpd"`
	Risk          string        `json:"risk"` // Low | Medium | High | Critical
	LastUpdated   string        `json:"lastUpdated"`
}

type PortfolioAiInsight struct {
	Headline     string      `json:"headline"`
	Body         string      `json:"body"`
	CtaLabel     string      `json:"ctaLabel"`
	Observations []AiInsight `json:"observations"`
}

type PortfolioData struct {
	Breadcrumb             []string            `json:"breadcrumb"`
	Title                  string              `json:"title"`
	Subtitle               string              `json:"subtitle"`
	Quote                  string              `json:"quote"`
	QuoteAttribution       string              `json:"quoteAttribution"`
	AsOfLabel              string              `json:"asOfLabel"`
	Kpis                   PortfolioKpis       `json:"kpis"`
	Filters                []FilterOption      `json:"filters"`
	Tabs                   []PortfolioTab      `json:"tabs"`
	SortLabel              string              `json:"sortLabel"`
	Customers              []CustomerRow       `json:"customers"`
	TotalCustomers         int                 `json:"totalCustomers"`
	TotalPages             int                 `json:"totalPages"`
	CurrentPage            int                 `json:"currentPage"`
	RowsPerPage            int                 `json:"rowsPerPage"`
	AiInsight              PortfolioAiInsight  `json:"aiInsight"`
	LoanExposure           LoanProductExposure `json:"loanExposure"`
	FooterQuote            string              `json:"footerQuote"`
	FooterQuoteAttribution string              `json:"footerQuoteAttribution"`
}

// --- Customer 360 ---

type CustomerProfile struct {
	Name               string   `json:"name"`
	Initials           string   `json:"initials"`
	RiskTag            string   `json:"riskTag"`
	Segment            string   `json:"segment"`
	Products           string   `json:"products"`
	CustomerID         string   `json:"customerId"`
	SinceYear          string   `json:"sinceYear"`
	Gender             string   `json:"gender,omitempty"`
	Age                *int     `json:"age,omitempty"`
	Phone              string   `json:"phone"`
	Email              string   `json:"email"`
	Address            string   `json:"address"`
	Occupation         string   `json:"occupation"`
	MonthlyIncomeLabel string   `json:"monthlyIncomeLabel"`
	CustomerSinceLabel string   `json:"customerSinceLabel"`
	Tags               []string `json:"tags"`
}

type HealthScoreFactor struct {
	Label string `json:"label"`
	Score int    `json:"score"`
}

type FinancialHealthScore struct {
	Score       int                 `json:"score"`
	Band        string              `json:"band"`
	DeltaPoints *int                `json:"deltaPoints,omitempty"`
	DeltaLabel  string              `json:"deltaLabel,omitempty"`
	Factors     []HealthScoreFactor `json:"factors"`
}

type CustomerAiInsight struct {
	Tag      string `json:"tag"`
	Body     string `json:"body"`
	CtaLabel string `json:"ctaLabel"`
}

type CustomerStat struct {
	Label string `json:"label"`
	Value string `json:"value"`
	Icon  string `json:"icon"` // exposure | loans | emi | balance | income | expenses
}

type ActiveLoanRow struct {
	Product     string `json:"product"`
	Sanctioned  string `json:"sanctioned"`
	Outstanding string `json:"outstanding"`
	EmiMonthly  string `json:"emiMonthly"`
	Roi         string `json:"roi"`
	NextDue     string `json:"nextDue"`
}

type CreditProfile struct {
	CibilScore            int      `json:"cibilScore"`
	CibilBand             string   `json:"cibilBand"`
	CibilDeltaPoints      *int     `json:"cibilDeltaPoints,omitempty"`
	CibilDeltaLabel       string   `json:"cibilDeltaLabel,omitempty"`
	UtilizationPct        int      `json:"utilizationPct"`
	UtilizationBand       string   `json:"utilizationBand"`
	UtilizationDeltaPct   *float64 `json:"utilizationDeltaPct,omitempty"`
	UtilizationDeltaLabel string   `json:"utilizationDeltaLabel,omitempty"`
	TotalActiveAccounts   int      `json:"totalActiveAccounts"`
	TotalEmiObligations   string   `json:"totalEmiObligations"`
	CurrentDpd            int      `json:"currentDpd"`
	HighestDpd12m         int      `json:"highestDpd12m"`
}

type ActivityItem struct {
	ID          string `json:"id"`
	Date        string `json:"date"`
	Description string `json:"description"`
	AmountLabel string `json:"amountLabel,omitempty"`
	Tone        string `json:"tone"` // good | bad | neutral
}

type TrendPoint struct {
	Label string  `json:"label"`
	Value float64 `json:"value"`
}

type BalanceTrend struct {
	RangeLabel      string       `json:"rangeLabel"`
	Points          []TrendPoint `json:"points"`
	AvgBalanceLabel string       `json:"avgBalanceLabel"`
	DeltaPct        *float64     `json:"deltaPct,omitempty"`
	DeltaLabel      string       `json:"deltaLabel,omitempty"`
}

type CashFlowMonth struct {
	Label   string  `json:"label"`
	Inflow  float64 `json:"inflow"`
	Outflow float64 `json:"outflow"`
	Net     float64 `json:"net"`
}

type CashFlowSeries struct {
	RangeLabel string          `json:"rangeLabel"`
	Months     []CashFlowMonth `json:"months"`
}

type RiskScoreTrend struct {
	RangeLabel string       `json:"rangeLabel"`
	Points     []TrendPoint `json:"points"`
}

type RiskInsightPoint struct {
	Headline string `json:"headline"`
	Detail   string `json:"detail"`
}

type Customer360Data struct {
	Breadcrumb     []string             `json:"breadcrumb"`
	BusinessUnit   string               `json:"businessUnit"`
	AsOfLabel      string               `json:"asOfLabel"`
	Profile        CustomerProfile      `json:"profile"`
	HealthScore    FinancialHealthScore `json:"healthScore"`
	AiInsight      CustomerAiInsight    `json:"aiInsight"`
	Stats          []CustomerStat       `json:"stats"`
	ActiveLoans    []ActiveLoanRow      `json:"activeLoans"`
	CreditProfile  CreditProfile        `json:"creditProfile"`
	RecentActivity []ActivityItem       `json:"recentActivity"`
	BalanceTrend   BalanceTrend         `json:"balanceTrend"`
	CashFlow       CashFlowSeries       `json:"cashFlow"`
	RiskScoreTrend RiskScoreTrend       `json:"riskScoreTrend"`
	RiskInsights   []RiskInsightPoint   `json:"riskInsights"`
}

// --- Cash Flow Intelligence ---

type CashFlowCustomerSummary struct {
	Name       string `json:"name"`
	Initials   string `json:"initials"`
	CustomerID string `json:"customerId"`
	Segment    string `json:"segment"`
	Age        *int   `json:"age,omitempty"`
	City       string `json:"city"`
	Products   string `json:"products"`
	RiskTag    string `json:"riskTag"`
}

type CashFlowKpi struct {
	Label          string   `json:"label"`
	Value          string   `json:"value"`
	DeltaPct       *float64 `json:"deltaPct,omitempty"`
	DeltaDirection string   `json:"deltaDirection,omitempty"` // up | down
	DeltaLabel     string   `json:"deltaLabel"`
	Icon           string   `json:"icon"` // income | expenses | emi | creditCard | obligations | netCashFlow
	Tone           string   `json:"tone"` // good | watch | risk
}

type CashFlowTrendMonth struct {
	Label              string  `json:"label"`
	SalaryInflow       float64 `json:"salaryInflow"`
	OtherInflow        float64 `json:"otherInflow"`
	Expenses           float64 `json:"expenses"`
	EmiPayments        float64 `json:"emiPayments"`
	CreditCardPayments float64 `json:"creditCardPayments"`
	Investments        float64 `json:"investments"`
	NetCashFlow        float64 `json:"netCashFlow"`
}

type CashFlowTrendSeries struct {
	RangeLabel string               `json:"rangeLabel"`
	Months     []CashFlowTrendMonth `json:"months"`
}

type BreakdownRow struct {
	Label       string  `json:"label"`
	AmountLabel string  `json:"amountLabel"`
	Pct         float64 `json:"pct"`
	Tone        string  `json:"tone"` // inflow | outflow | investment | total
	Bold        bool    `json:"bold,omitempty"`
}

type CashFlowBreakdown struct {
	MonthLabel string         `json:"monthLabel"`
	Rows       []BreakdownRow `json:"rows"`
}

type CashFlowAiInsight struct {
	Body     string   `json:"body"`
	Factors  []string `json:"factors"`
	CtaLabel string   `json:"ctaLabel"`
}

type ForecastMonth struct {
	Label                 string  `json:"label"`
	Sublabel              string  `json:"sublabel"`
	ExpectedIncome        float64 `json:"expectedIncome"`
	ExpectedObligations   float64 `json:"expectedObligations"`
	ProjectedFreeCashFlow float64 `json:"projectedFreeCashFlow"`
}

type CashFlowForecast struct {
	RangeLabel string          `json:"rangeLabel"`
	Months     []ForecastMonth `json:"months"`
}

type Next30Days struct {
	ExpectedIncomeLabel        string `json:"expectedIncomeLabel"`
	ExpectedObligationsLabel   string `json:"expectedObligationsLabel"`
	ProjectedFreeCashFlowLabel string `json:"projectedFreeCashFlowLabel"`
}

type TransactionInsightRow struct {
	Icon       string  `json:"icon"` // salary | card | upi | sip
	Value      string  `json:"value"`
	Label      string  `json:"label"`
	DeltaPct   float64 `json:"deltaPct"`
	DeltaLabel string  `json:"deltaLabel"`
}

type CashFlowIntelligenceData struct {
	Breadcrumb          []string                `json:"breadcrumb"`
	Title               string                  `json:"title"`
	Subtitle            string                  `json:"subtitle"`
	Quote               string                  `json:"quote"`
	QuoteAttribution    string                  `json:"quoteAttribution"`
	AsOfLabel           string                  `json:"asOfLabel"`
	Customer            CashFlowCustomerSummary `json:"customer"`
	Kpis                []CashFlowKpi           `json:"kpis"`
	Trend               CashFlowTrendSeries     `json:"trend"`
	Breakdown           CashFlowBreakdown       `json:"breakdown"`
	AiInsight           CashFlowAiInsight       `json:"aiInsight"`
	Forecast            CashFlowForecast        `json:"forecast"`
	Next30Days          Next30Days              `json:"next30Days"`
	TransactionInsights []TransactionInsightRow `json:"transactionInsights"`
}

// --- Credit & Risk Intelligence ---

type CreditRiskProductChip struct {
	Label      string `json:"label"`
	ValueLabel string `json:"valueLabel"`
	Icon       string `json:"icon"` // homeLoan | personalLoan
}

type RiskMigrationStage struct {
	Label     string `json:"label"`
	Tone      string `json:"tone"` // low | moderate | high
	DateLabel string `json:"dateLabel"`
}

type RiskMigration struct {
	Stages      []RiskMigrationStage `json:"stages"`
	Description string               `json:"description"`
}

type TrendSeries struct {
	RangeLabel string       `json:"rangeLabel"`
	Points     []TrendPoint `json:"points"`
}

type DonutSlice struct {
	Label string  `json:"label"`
	Value float64 `json:"value"`
	Pct   float64 `json:"pct"`
	Color string  `json:"color"`
}

type ActiveAccounts struct {
	Total  int          `json:"total"`
	Slices []DonutSlice `json:"slices"`
}

type BureauSummaryRow struct {
	Label      string `json:"label"`
	Value      string `json:"value"`
	DeltaLabel string `json:"deltaLabel,omitempty"`
	Tone       string `json:"tone,omitempty"` // default | down
}

type BureauSummary struct {
	SourceLabel string             `json:"sourceLabel"`
	Rows        []BureauSummaryRow `json:"rows"`
}

type RiskDriver struct {
	Headline string `json:"headline"`
	Detail   string `json:"detail"`
	Impact   string `json:"impact"` // High Impact | Medium Impact
}

type PeerComparisonMetric struct {
	Label         string  `json:"label"`
	CustomerValue float64 `json:"customerValue"`
	IndustryValue float64 `json:"industryValue"`
	CustomerLabel string  `json:"customerLabel"`
	IndustryLabel string  `json:"industryLabel"`
	MaxScale      float64 `json:"maxScale"`
}

type PeerComparison struct {
	CustomerLegend string                 `json:"customerLegend"`
	IndustryLegend string                 `json:"industryLegend"`
	Metrics        []PeerComparisonMetric `json:"metrics"`
}

type CreditAiInsight struct {
	Body     string   `json:"body"`
	Actions  []string `json:"actions"`
	CtaLabel string   `json:"ctaLabel"`
}

type CreditRiskIntelligenceData struct {
	Breadcrumb       []string                `json:"breadcrumb"`
	Title            string                  `json:"title"`
	Subtitle         string                  `json:"subtitle"`
	AsOfLabel        string                  `json:"asOfLabel"`
	Customer         CashFlowCustomerSummary `json:"customer"`
	ProductChips     []CreditRiskProductChip `json:"productChips"`
	HealthScore      FinancialHealthScore    `json:"healthScore"`
	ScoreTrend       TrendSeries             `json:"scoreTrend"`
	RiskMigration    RiskMigration           `json:"riskMigration"`
	DpdTrend         TrendSeries             `json:"dpdTrend"`
	UtilizationTrend TrendSeries             `json:"utilizationTrend"`
	ActiveAccounts   ActiveAccounts          `json:"activeAccounts"`
	BureauSummary    BureauSummary           `json:"bureauSummary"`
	RiskDrivers      []RiskDriver            `json:"riskDrivers"`
	PeerComparison   PeerComparison          `json:"peerComparison"`
	AiInsight        CreditAiInsight         `json:"aiInsight"`
}

// --- Early Warnings ---

type WarningKpi struct {
	Label    string `json:"label"`
	Value    string `json:"value"`
	SubLabel string `json:"subLabel"`
	Tone     string `json:"tone"` // critical | risk | watch | good
}

type WarningSignal struct {
	Label     string `json:"label"`
	Direction string `json:"direction"` // up | down
	Value     string `json:"value,omitempty"`
}

type FeaturedAlert struct {
	Tag           string          `json:"tag"`
	Severity      string          `json:"severity"` // Critical | High | Medium
	CustomerName  string          `json:"customerName"`
	CustomerID    string          `json:"customerId"`
	UserID        string          `json:"userId"`
	Product       string          `json:"product"`
	ExposureLabel string          `json:"exposureLabel"`
	CreditScore   int             `json:"creditScore"`
	RiskTag       string          `json:"riskTag"`
	Narrative     string          `json:"narrative"`
	Signals       []WarningSignal `json:"signals"`
	RiskImpact    string          `json:"riskImpact"`
	DetectedLabel string          `json:"detectedLabel"`
}

type AlertCardData struct {
	ID            string `json:"id"`
	Icon          string `json:"icon"` // emi | salary | utilization | delay | bureau | borrowing | withdrawal
	Title         string `json:"title"`
	Severity      string `json:"severity"`
	CustomerName  string `json:"customerName"`
	CustomerID    string `json:"customerId"`
	UserID        string `json:"userId"`
	Product       string `json:"product"`
	ExposureLabel string `json:"exposureLabel"`
	CreditScore   int    `json:"creditScore"`
	Description   string `json:"description"`
	DetectedLabel string `json:"detectedLabel"`
}

type RiskTrendPoint struct {
	Label string `json:"label"`
	Score int    `json:"score"`
}

type RiskTrend struct {
	CurrentScore int              `json:"currentScore"`
	Points       []RiskTrendPoint `json:"points"`
}

type RecentAlertItem struct {
	ID           string `json:"id"`
	Label        string `json:"label"`
	CustomerName string `json:"customerName"`
	TimeAgo      string `json:"timeAgo"`
	Severity     string `json:"severity"`
}

type EarlyWarningsData struct {
	Breadcrumb    []string           `json:"breadcrumb"`
	Title         string             `json:"title"`
	Subtitle      string             `json:"subtitle"`
	AsOfLabel     string             `json:"asOfLabel"`
	Kpis          []WarningKpi       `json:"kpis"`
	Filters       []FilterOption     `json:"filters"`
	FeaturedAlert *FeaturedAlert     `json:"featuredAlert"`
	OtherAlerts   []AlertCardData    `json:"otherAlerts"`
	AiInsight     PortfolioAiInsight `json:"aiInsight"`
	RiskTrend     RiskTrend          `json:"riskTrend"`
	RecentAlerts  []RecentAlertItem  `json:"recentAlerts"`
}
