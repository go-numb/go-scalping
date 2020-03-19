package bitflyer

import (
	"context"
	"log"
	"os"
	"sync"

	"github.com/mxmCherry/movavg"

	"github.com/go-numb/go-exchanges/api/bitflyer/v1/realtime/jsonrpc"
	"github.com/go-numb/go-exchanges/api/bitflyer/v1/types"

	"github.com/go-numb/go-bitflyer-wrapper/executions"
	"github.com/go-numb/go-bitflyer-wrapper/orders"
	v1 "github.com/go-numb/go-exchanges/api/bitflyer/v1"
)

type Client struct {
	C *v1.Client

	SFD *SFDer

	SE *executions.Execution
	SO *orders.Managed

	FE *executions.Execution
	FO *orders.Managed

	MA movavg.Multi
}

func New() *Client {
	key := os.Getenv("BFKEY")
	secret := os.Getenv("BFSECRET")

	return &Client{
		C: v1.New(&v1.Config{
			Key:    key,
			Secret: secret,
		}),

		SFD: new(SFDer),

		SE: executions.New(),
		SO: orders.New(),
		FE: executions.New(),
		FO: orders.New(),

		MA: movavg.Multi{
			movavg.NewSMA(9),
			movavg.NewSMA(21),
		},
	}
}

func (p *Client) Connect(ctx context.Context, l *log.Logger) {
	ch := make(chan jsonrpc.Response)

	channels := []string{
		"lightning_executions",
	}
	symbols := []string{
		string(types.BTCJPY),
		string(types.FXBTCJPY),
	}
	go jsonrpc.Connect(ctx, ch, channels, symbols, l)

	channels = []string{
		"lightning_ticker_FX_BTC_JPY",
		"child_order_events",
	}
	go jsonrpc.ConnectForPrivate(ctx, ch, p.C.Config().Key, p.C.Config().Secret, channels, l)

	for {
		select {
		case v := <-ch:
			switch v.ProductCode {
			case types.BTCJPY:
				switch v.Types {
				case jsonrpc.Executions:
					p.SE.Set(v.Executions)
					p.SFD.Culc(p.SE.LTP(), p.FE.LTP())

				case jsonrpc.ChildOrders:
					p.SO.Switch(v.ChildOrderEvents)
				}

			case types.FXBTCJPY:
				switch v.Types {
				case jsonrpc.Executions:
					p.FE.Set(v.Executions)
					p.SFD.Culc(p.SE.LTP(), p.FE.LTP())

				case jsonrpc.ChildOrders:
					p.FO.Switch(v.ChildOrderEvents)
				}
			}
		}
	}
}

type SFDer struct {
	sync.RWMutex

	ratio float64
}

func (p *SFDer) Culc(s, f float64) {
	p.Lock()
	defer p.Unlock()

	p.ratio = f / s
}

func (p *SFDer) Ratio() float64 {
	p.RLock()
	defer p.RUnlock()

	return p.ratio
}
