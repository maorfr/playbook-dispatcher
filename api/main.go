package api

import (
	"context"
	"fmt"
	"net/http"
	"playbook-dispatcher/api/controllers"
	"playbook-dispatcher/utils"

	"go.uber.org/zap"

	"github.com/deepmap/oapi-codegen/pkg/middleware"
	"github.com/getkin/kin-openapi/openapi3"
	echoPrometheus "github.com/globocom/echo-prometheus"
	"github.com/labstack/echo/v4"
	echoMiddleware "github.com/labstack/echo/v4/middleware"
	"github.com/redhatinsights/platform-go-middlewares/identity"
	"github.com/redhatinsights/platform-go-middlewares/request_id"
	"github.com/spf13/viper"
)

const specFile = "/api/playbook-dispatcher/v1/openapi.json"
const requestIdHeader = "x-rh-insights-request-id"

func init() {
	openapi3.DefineStringFormat("uuid", `^[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89aAbB][a-f0-9]{3}-[a-f0-9]{12}$`)
	openapi3.DefineStringFormat("url", `^https?:\/\/.*$`)
}

func Start(cfg *viper.Viper, log *zap.SugaredLogger, errors chan error, ready, live *utils.ProbeHandler) func(ctx context.Context) {
	db, sql := connectToDatabase(cfg)

	ready.Register(sql.Ping)
	live.Register(sql.Ping)

	spec, err := controllers.GetSwagger()
	utils.DieOnError(err)

	server := echo.New()
	server.HideBanner = true
	server.Debug = false

	server.Use(
		echoPrometheus.MetricsMiddleware(),
		echoMiddleware.BodyLimit(cfg.GetString("http.max.body.size")),
		echo.WrapMiddleware(request_id.ConfiguredRequestID(requestIdHeader)),
	)

	server.GET(specFile, func(ctx echo.Context) error {
		return ctx.JSON(http.StatusOK, spec)
	})

	ctrl := controllers.CreateControllers(db, log)

	internal := server.Group("/internal/*")
	public := server.Group("/api/playbook-dispatcher/v1/*")

	internal.Use(middleware.OapiRequestValidator(spec))
	controllers.RegisterHandlers(internal, ctrl)

	public.Use(echo.WrapMiddleware(identity.EnforceIdentity))
	public.Use(middleware.OapiRequestValidator(spec))
	controllers.RegisterHandlers(public, ctrl)

	go func() {
		errors <- server.Start(fmt.Sprintf("0.0.0.0:%d", cfg.GetInt("web.port")))
	}()

	return func(ctx context.Context) {
		log.Info("Shutting down web server")
		utils.StopServer(server, ctx, log)

		if sqlConnection, err := db.DB(); err != nil {
			sqlConnection.Close()
		}
	}
}
