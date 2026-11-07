package desktop

import (
	"github.com/egoist/mygo/ui"
	"math"
	"strconv"
	"strings"
)

func fileRateButton(c *ui.Context, f *fileState) {
	current, label := "不限速", "限速"
	if rate := f.limiter.Rate(); rate > 0 {
		current = bytesLabel(rate) + "/s"
		label += " · " + current
	}
	button := ui.Button(c, label).Label("设置传输限速").Tooltip("当前：" + current).OnClick(func() {
		f.rateOpen = !f.rateOpen
	})
	ui.Popover(c, button, &f.rateOpen, func() {
		ui.Column(c).Width(300).Padding(6).Gap(6).Children(func() {
			ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
				windowHeading(c, "传输限速").Grow(1)
				flatButton(c, "关闭限速设置", closeIcon, "").OnClick(func() { f.rateOpen = false })
			})
			fileRateControls(c, f)
			ui.Text(c, "仅影响当前文件标签。SFTP / FTP 传输中即时生效，SCP / rsync 下次传输生效。").FontSize(11).TextColor(c.Theme().TextMuted).Wrap()
		})
	})
}

func fileRateControls(c *ui.Context, f *fileState) {
	if f.rateChoice == "" {
		f.rateChoice = "不限速"
	}
	ui.Row(c).Gap(6).Children(func() {
		old := f.rateChoice
		ui.Select(c, &f.rateChoice, []string{"不限速", "128 KiB/s", "512 KiB/s", "1 MiB/s", "5 MiB/s", "10 MiB/s", "自定义"}).Label("传输限速").Width(155)
		if old != f.rateChoice {
			rates := map[string]int64{"不限速": 0, "128 KiB/s": 128 << 10, "512 KiB/s": 512 << 10, "1 MiB/s": 1 << 20, "5 MiB/s": 5 << 20, "10 MiB/s": 10 << 20}
			if rate, ok := rates[f.rateChoice]; ok {
				f.limiter.Set(rate)
				f.rateError = ""
			}
		}
	})
	if f.rateChoice == "自定义" {
		ui.Row(c).Gap(6).Children(func() {

			if f.rateUnit == "" {
				f.rateUnit = "KiB/s"
			}
			ui.Select(c, &f.rateUnit, []string{"KiB/s", "MiB/s"}).Width(90).MinWidth(90)
			input := ui.TextInput(c, &f.rateCustom).Label("自定义速率").Width(90)
			apply := func() {
				n, err := parseTransferRate(f.rateCustom, f.rateUnit)
				if err != nil {
					f.rateError = "请输入有效速率：0 为不限速，非零范围 1 KiB/s–1024 MiB/s"
					return
				}
				f.limiter.Set(n)
				f.rateError = ""
			}
			if input.Submitted() {
				apply()
			}
			ui.Button(c, "应用限速").OnClick(apply)
		})
	}

	if f.rateError != "" {
		ui.Text(c, f.rateError).TextColor(c.Theme().Danger).Wrap()
	}
}

func parseTransferRate(text, unit string) (int64, error) {
	n, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
	scale := float64(1024)
	if unit == "MiB/s" {
		scale = 1048576
	} else if unit != "KiB/s" {
		return 0, strconv.ErrSyntax
	}
	bytes := n * scale
	if err != nil || math.IsNaN(bytes) || math.IsInf(bytes, 0) || bytes < 0 || bytes > 1<<30 || bytes > 0 && bytes < 1024 {
		return 0, strconv.ErrRange
	}
	return int64(math.Round(bytes)), nil
}
