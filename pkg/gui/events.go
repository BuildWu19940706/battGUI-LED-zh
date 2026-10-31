package gui

import (
	"context"
	"fmt"

	"github.com/sirupsen/logrus"

	"github.com/charlie0129/batt/pkg/calibration"
	"github.com/charlie0129/batt/pkg/events"
)

func (c *menuController) subscribeEvents(ctx context.Context) {
	for event := range c.api.SubscribeEvents(ctx) {
		logrus.WithFields(logrus.Fields{
			"event": event.Name,
			"data":  string(event.Data),
		}).Debug("new event")

		switch event.Name {
		case events.CalibrationAction:
			payload, err := events.DecodeAs[events.CalibrationActionEvent](event)
			if err != nil {
				logrus.WithError(err).Error("failed to decode calibration.action event")
				continue
			}
			body := payload.Message
			if activeLocale == "zh-Hans" {
				body = fmt.Sprintf(tr("Calibration action: %s"), tr(payload.Action))
				if payload.Action == "ScheduleError" {
					body += "\n" + payload.Message
				}
			}
			showNotification("Calibration", body)
		case events.CalibrationPhase:
			payload, err := events.DecodeAs[events.CalibrationPhaseEvent](event)
			if err != nil {
				logrus.WithError(err).Error("failed to decode calibration.phase event")
				continue
			}
			if calibrationPhaseNotifies(calibration.Phase(payload.To)) {
				body := payload.Message
				if activeLocale == "zh-Hans" {
					body = fmt.Sprintf(tr("Calibration phase: %s"), tr(payload.To))
					if payload.To == "Error" {
						body += "\n" + payload.Message
					}
				}
				showNotification("Calibration", body)
			}
		case events.ChargeOnceAction:
			payload, err := events.DecodeAs[events.ChargeOnceActionEvent](event)
			if err != nil {
				logrus.WithError(err).Error("failed to decode charge-once.action event")
				continue
			}
			body := payload.Message
			if activeLocale == "zh-Hans" {
				switch payload.Action {
				case "Start":
					body = fmt.Sprintf(tr("One-time charge to %d%% started."), payload.Target)
				case "Cancel":
					body = tr("One-time charge cancelled. Your normal charge limit applies again.")
				case "Complete":
					body = tr("One-time charge completed. Your normal charge limit applies again.")
				}
			}
			showNotification("One-Time Charge", body)
		}
	}
}

func calibrationPhaseNotifies(phase calibration.Phase) bool {
	switch phase {
	case calibration.PhaseDischarge,
		calibration.PhaseCharge,
		calibration.PhaseHold,
		calibration.PhasePostHold,
		calibration.PhaseRestore,
		calibration.PhaseError:
		return true
	default:
		return false
	}
}
