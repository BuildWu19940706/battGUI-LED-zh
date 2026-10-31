package gui

import (
	"fmt"
	"github.com/charlie0129/batt/pkg/powerinfo"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestPreferredLanguageSelection(t *testing.T) {
	for _, tc := range []struct{ in, want string }{{"zh-Hans-CN", "zh-Hans"}, {"zh_CN", "zh-Hans"}, {"zh-SG", "zh-Hans"}, {"zh-Hant-CN", "en"}, {"zh-TW", "en"}, {"en-US", "en"}, {"ja-JP", "en"}, {"", "en"}} {
		if got := selectLocale(tc.in); got != tc.want {
			t.Fatalf("locale(%q)=%q want %q", tc.in, got, tc.want)
		}
	}
}

func TestChineseDynamicGUI(t *testing.T) {
	previous := activeLocale
	activeLocale = "zh-Hans"
	t.Cleanup(func() { activeLocale = previous })
	if got := chargeOnceLimitTitle(80); got != "立即充至 80%" {
		t.Fatal(got)
	}
	if got := temporaryDisableCountdownTitle(80, time.Hour); got != "恢复 80% 上限还需 1 小时" {
		t.Fatal(got)
	}
	if got := controlStatusTitle(&powerinfo.ControlStatus{State: "reached", TargetPercent: 80, ChargePercent: 80}); !strings.Contains(got, "已达上限") {
		t.Fatal(got)
	}
	if got := ledStatusTitle(powerinfo.LEDStatus{Reason: "firmware_unverified"}); !strings.Contains(got, "尚未验证") {
		t.Fatal(got)
	}
}

func TestTranslationFormatAndCoverage(t *testing.T) {
	verbs := regexp.MustCompile(`%(?:[-+0-9.]*l?[dsfcv@]|%)`)
	for key, value := range chineseMessages {
		if value == "" {
			t.Errorf("empty translation: %q", key)
		}
		a, b := verbs.FindAllString(key, -1), verbs.FindAllString(value, -1)
		if fmt.Sprint(a) != fmt.Sprint(b) {
			t.Errorf("format mismatch %q: %v vs %v", key, a, b)
		}
	}
	for _, key := range []string{"Quick Limits", "Quit Menubar App", "Charge Range...", "Charging", "Cancel", "Save", "MagSafe: %s", "Upper limit (%)", "Lower limit (%)"} {
		if chineseMessages[key] == "" {
			t.Errorf("missing UI translation %q", key)
		}
	}
	if got := translate("en", "Quick Limits"); got != "Quick Limits" {
		t.Fatal(got)
	}
}

func TestRangeInputValidation(t *testing.T) {
	for _, tc := range []struct {
		upper, lower int
		ok           bool
	}{{80, 78, true}, {70, 60, true}, {80, 80, false}, {101, 78, false}, {80, 9, false}} {
		if got := validChargeRange(tc.upper, tc.lower); got != tc.ok {
			t.Fatalf("range %d/%d valid=%v", tc.lower, tc.upper, got)
		}
	}
}
