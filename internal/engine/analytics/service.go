package analytics

type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) GetClickHistory(linkID string, start, end int64, limit, offset int) ([]ClickStat, error) {
	return s.repo.GetClicks(linkID, start, end, limit, offset)
}

func (s *Service) GetStatsOverview(linkID string, startDate, endDate string) ([]DailyStat, error) {
	return s.repo.GetDailyStats(linkID, startDate, endDate)
}
