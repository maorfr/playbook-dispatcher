package responseConsumer

import (
	"context"
	"playbook-dispatcher/internal/common/constants"
	kafkaUtils "playbook-dispatcher/internal/common/kafka"
	"playbook-dispatcher/internal/common/model/db"
	"playbook-dispatcher/internal/common/model/message"
	"playbook-dispatcher/internal/common/utils"
	"playbook-dispatcher/internal/response-consumer/instrumentation"

	"go.uber.org/zap"

	k "github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/google/uuid"

	"gorm.io/gorm"
)

const (
	EventPlaybookOnStats = "playbook_on_stats"
	EventRunnerOnFailed  = "runner_on_failed"
)

type handler struct {
	db  *gorm.DB
	log *zap.SugaredLogger
}

func (this *handler) onMessage(msg *k.Message) {
	requestId, correlationId, err := getHeaders(msg)
	if err != nil {
		// TODO: get ctx as onMessage param
		instrumentation.CannotReadHeaders(this.log, err)
		return
	}

	ctx, log := utils.WithRequestId(utils.SetLog(context.Background(), this.log.With("correlation_id", correlationId)), requestId)

	value := &message.PlaybookRunResponseMessageYaml{}

	if err := value.UnmarshalJSON(msg.Value); err != nil {
		instrumentation.UnmarshallIncomingMessageError(ctx, err)
		return
	}

	log.Debugw("Processing message", "account", value.Account, "upload_timestamp", value.UploadTimestamp)

	status := inferStatus(&value.Events)

	queryBuilder := this.db.Model(db.Run{}).
		Select("status").
		Where("account = ?", value.Account).
		Where("correlation_id = ?", correlationId)

	result := queryBuilder.Updates(db.Run{Status: status})

	if result.Error != nil {
		instrumentation.PlaybookRunUpdateError(ctx, result.Error, value.Account, status, correlationId)
	} else if result.RowsAffected > 0 {
		instrumentation.PlaybookRunUpdated(ctx, value.Account, status, correlationId)
	} else {
		instrumentation.PlaybookRunUpdateMiss(ctx, value.Account, status, correlationId)
	}
}

func inferStatus(events *[]message.PlaybookRunResponseMessageYamlEventsElem) string {
	finished := false
	failed := false

	for _, event := range *events {
		if event.Event == EventPlaybookOnStats {
			finished = true
		}

		if event.Event == EventRunnerOnFailed {
			failed = true
		}
	}

	switch {
	case finished && failed:
		return db.RunStatusFailure
	case finished && !failed:
		return db.RunStatusSuccess
	default:
		return db.RunStatusRunning
	}
}

func getHeaders(msg *k.Message) (requestId string, correlationId uuid.UUID, err error) {
	if requestId, err = kafkaUtils.GetHeader(msg, constants.HeaderRequestId); err != nil {
		return
	}

	var correlationIdRaw string
	if correlationIdRaw, err = kafkaUtils.GetHeader(msg, constants.HeaderCorrelationId); err != nil {
		return
	}

	if correlationId, err = uuid.Parse(correlationIdRaw); err != nil {
		return
	}

	return
}
