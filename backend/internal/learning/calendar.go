package learning

import (
	"context"
	"time"
)

// CalendarProgress uses the same task identities and completion rules as TodayHub.
// Records and tasks are fetched once per period rather than once per calendar cell.
func (s *Service) CalendarProgress(ctx context.Context, groupID, userID uint64, month string, settings map[string]any, loc *time.Location) (map[string]TodayProgress, error) {
	start, err := time.ParseInLocation("2006-01-02", month+"-01", loc)
	if err != nil {
		return nil, err
	}
	end := start.AddDate(0, 1, -1)
	weeks, err := s.repo.ListWeeks(ctx, groupID)
	if err != nil {
		return nil, err
	}
	ids := []uint64{}
	for _, week := range weeks {
		if week.StartDate <= end.Format("2006-01-02") && week.EndDate >= start.Format("2006-01-02") {
			ids = append(ids, week.ID)
		}
	}
	rawTasks, err := s.repo.ListTasksForWeeks(ctx, groupID, ids)
	if err != nil {
		return nil, err
	}
	byWeek := map[uint64][]Task{}
	for _, task := range rawTasks {
		byWeek[task.WeekID] = append(byWeek[task.WeekID], task)
	}
	periodRecords := map[string][]TodayRecord{}
	progress := map[string]TodayProgress{}
	for day := start; !day.After(end); day = day.AddDate(0, 0, 1) {
		date := day.Format("2006-01-02")
		var current *Week
		for i := range weeks {
			week := &weeks[i]
			if week.StartDate <= date && week.EndDate >= date && (current == nil || week.StartDate > current.StartDate) {
				current = week
			}
		}
		from, to := start.Format("2006-01-02"), end.Format("2006-01-02")
		var weekMap map[string]any
		var tasks []map[string]any
		if current != nil {
			from, to = current.StartDate, current.EndDate
			weekMap, tasks = WeekMap(*current), activeWeekTaskMaps(byWeek[current.ID])
		}
		verseFrom, verseTo := verseRecordRange(settings, date, from, to)
		key := from + ":" + to + ":" + verseFrom + ":" + verseTo
		records, loaded := periodRecords[key]
		if !loaded {
			records, err = s.completionRecords(ctx, groupID, userID, TodayContent{Date: date, Settings: settings, RecordFrom: from, RecordTo: to})
			if err != nil {
				return nil, err
			}
			periodRecords[key] = records
		}
		visible := make([]TodayRecord, 0, len(records))
		for _, record := range records {
			if record.LogicalDate <= date {
				visible = append(visible, record)
			}
		}
		progress[date] = taskProgress(buildTodayTasks(date, weekMap, tasks, settings, visible))
	}
	return progress, nil
}

func taskProgress(tasks []TodayTaskVO) TodayProgress {
	progress := TodayProgress{Total: len(tasks)}
	for _, task := range tasks {
		if task.Completed {
			progress.Completed++
		}
	}
	if progress.Total > 0 {
		progress.Percent = (progress.Completed*100 + progress.Total/2) / progress.Total
	}
	return progress
}

func (s *Service) completionRecords(ctx context.Context, groupID, userID uint64, content TodayContent) ([]TodayRecord, error) {
	records, err := s.repo.ListCompletionRecords(ctx, groupID, userID, content.RecordFrom, content.RecordTo)
	if err != nil {
		return nil, err
	}
	from, to := verseRecordRange(content.Settings, content.Date, content.RecordFrom, content.RecordTo)
	if from != content.RecordFrom || to != content.RecordTo {
		extra, err := s.repo.ListCompletionRecords(ctx, groupID, userID, from, to)
		if err != nil {
			return nil, err
		}
		for _, record := range extra {
			if record.TaskType == "daily_verse" {
				records = append(records, record)
			}
		}
	}
	return records, nil
}
