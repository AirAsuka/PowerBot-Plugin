package manager

import "testing"

func TestWeeklyReminderArgs(t *testing.T) {
	cronExpr, qqs, err := weeklyReminderArgs("周五、周一,三", "09", "30", "987654321，123456789 987654321")
	if err != nil {
		t.Fatal(err)
	}
	if cronExpr != "30 9 * * 1,3,5" {
		t.Fatalf("cron = %q", cronExpr)
	}
	if qqs != "123456789,987654321" {
		t.Fatalf("qqs = %q", qqs)
	}
}

func TestWeeklyReminderArgsRejectsInvalidValues(t *testing.T) {
	tests := [][]string{
		{"周八", "9", "30", "123"},
		{"周一", "24", "30", "123"},
		{"周一", "9", "60", "123"},
		{"周一", "9", "30", "abc"},
	}
	for _, args := range tests {
		if _, _, err := weeklyReminderArgs(args[0], args[1], args[2], args[3]); err == nil {
			t.Fatalf("weeklyReminderArgs(%q) should fail", args)
		}
	}
}
