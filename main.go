package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/go-numb/go-exchanges/api/bitflyer/v1/private/cancels"
	"github.com/go-numb/go-exchanges/api/bitflyer/v1/private/orders"
	"github.com/go-numb/go-exchanges/api/bitflyer/v1/types"

	"github.com/go-numb/go-scalping/api/bitflyer"
	"github.com/nsf/termbox-go"
)

var (
	code   string
	size   float64 = 0.01
	expire int     = 1

	diff      float64
	diffRatio float64

	f *os.File
)

func init() {
	flag.StringVar(&code, "code", "FX_BTC_JPY", "<-code> is product code, default FX_BTC_JPY")
	flag.Parse()

	f, _ = os.OpenFile("server.log", os.O_CREATE|os.O_APPEND|os.O_WRONLY, os.ModePerm)
}

func main() {
	c := bitflyer.New()

	ctx, cancel := context.WithCancel(context.Background())
	l := log.New(f, "websocket", log.Lmicroseconds)
	go c.Connect(ctx, l)

	var (
		eg      errgroup.Group
		loggers = NewLoggers()
	)

	eg.Go(func() error {
		if err := termbox.Init(); err != nil {
			panic(err)
		}
		defer termbox.Close()

		for {
			switch ev := termbox.PollEvent(); ev.Type {
			case termbox.EventKey:
				switch ev.Key {
				case termbox.KeyEsc: //ESCキーで終了
					cancel()
					return fmt.Errorf("push Escape key.")

				case termbox.KeyBackspace2: // Ctrl + _キー: 成買い
					o, err := c.C.ChildOrder(orders.NewForChildOrder(
						types.ProductCode(code),
						types.MARKET,
						types.BUY,
						types.GTC,
						0,
						size,
						expire,
					))
					if err != nil {
						loggers.Set(true, err.Error())
						continue
					}
					loggers.Set(false, o.ChildOrderAcceptanceId)

				case termbox.KeyEnter: // Ctrl + /キー: 成売り
					o, err := c.C.ChildOrder(orders.NewForChildOrder(
						types.ProductCode(code),
						types.MARKET,
						types.SELL,
						types.GTC,
						0,
						size,
						expire,
					))
					if err != nil {
						loggers.Set(true, err.Error())
						continue
					}
					loggers.Set(false, o.ChildOrderAcceptanceId)

				case termbox.KeyArrowUp: // ↑キー: BestBid買い
					_, bestbid := c.FE.Best()
					o, err := c.C.ChildOrder(orders.NewForChildOrder(
						types.ProductCode(code),
						types.LIMIT,
						types.BUY,
						types.GTC,
						bestbid,
						size,
						expire,
					))
					if err != nil {
						loggers.Set(true, err.Error())
						continue
					}
					loggers.Set(false, o.ChildOrderAcceptanceId)

				case termbox.KeyArrowDown: // ↓キー: BestAsk売り
					bestask, _ := c.FE.Best()
					o, err := c.C.ChildOrder(orders.NewForChildOrder(
						types.ProductCode(code),
						types.LIMIT,
						types.SELL,
						types.GTC,
						bestask,
						size,
						expire,
					))
					if err != nil {
						loggers.Set(true, err.Error())
						continue
					}
					loggers.Set(false, o.ChildOrderAcceptanceId)

				case termbox.KeyArrowRight: // →キー: -0.0x%指値買い
					_, bestbid := c.FE.Best()
					o, err := c.C.ChildOrder(orders.NewForChildOrder(
						types.ProductCode(code),
						types.LIMIT,
						types.BUY,
						types.GTC,
						bestbid-diff,
						size,
						expire,
					))
					if err != nil {
						loggers.Set(true, err.Error())
						continue
					}
					loggers.Set(false, o.ChildOrderAcceptanceId)

				case termbox.KeyArrowLeft: // ←キー: +0.0x%指値売り
					bestask, _ := c.FE.Best()
					o, err := c.C.ChildOrder(orders.NewForChildOrder(
						types.ProductCode(code),
						types.LIMIT,
						types.SELL,
						types.GTC,
						bestask+diff,
						size,
						expire,
					))
					if err != nil {
						loggers.Set(true, err.Error())
						continue
					}
					loggers.Set(false, o.ChildOrderAcceptanceId)

				case termbox.KeyCtrlS: // Ctrl+Sキー: SFD売り指値
					sfdASK := math.Ceil(c.SE.LTP() * 1.05)
					o, err := c.C.ChildOrder(orders.NewForChildOrder(
						types.ProductCode(code),
						types.LIMIT,
						types.SELL,
						types.GTC,
						sfdASK,
						size,
						expire,
					))
					if err != nil {
						loggers.Set(true, err.Error())
						continue
					}
					loggers.Set(false, o.ChildOrderAcceptanceId)

				case termbox.KeyCtrlA: // Ctrl+Aキー: SFD買い戻し
					sfdBID := math.Floor(c.SE.LTP() * 1.05)
					o, err := c.C.ChildOrder(orders.NewForChildOrder(
						types.ProductCode(code),
						types.LIMIT,
						types.SELL,
						types.IOC,
						sfdBID,
						size,
						expire,
					))
					if err != nil {
						loggers.Set(true, err.Error())
						continue
					}
					loggers.Set(false, o.ChildOrderAcceptanceId)

				case termbox.KeyCtrlL:
					size += 0.01
					loggers.Set(false, fmt.Sprintf("trade size up: %.2f", size))
				case termbox.KeyCtrlK:
					size -= 0.01
					loggers.Set(false, fmt.Sprintf("trade size down: %.2f", size))
				case termbox.KeyCtrlP:
					diffRatio += 0.0001
					diff = c.FE.LTP() * diffRatio
					loggers.Set(false, fmt.Sprintf("trade limit diff up: ¥%.f", diff))
				case termbox.KeyCtrlO:
					diffRatio -= 0.0001
					diff = c.FE.LTP() * diffRatio
					loggers.Set(false, fmt.Sprintf("trade limit diff down: ¥%.f", diff))

				case termbox.KeyCtrlR:
					_, _, sum := c.FO.Positions.Sum()
					way := types.BUY
					if -0.01 < sum && sum < 0.01 {
						continue
					} else if sum < 0 {
						way = types.SELL
					}
					o, err := c.C.ChildOrder(orders.NewForChildOrder(
						types.ProductCode(code),
						types.MARKET,
						way,
						types.GTC,
						0,
						sum,
						expire,
					))
					if err != nil {
						loggers.Set(true, err.Error())
						continue
					}
					loggers.Set(false, o.ChildOrderAcceptanceId)

				case termbox.KeySpace:
					if err := c.C.CancelAll(cancels.New(types.ProductCode(code))); err != nil {
						loggers.Set(true, err.Error())
						continue
					}
					loggers.Set(false, fmt.Sprintf("api limit: %d, orders: %d", c.C.Limit.Remain(false), c.C.Limit.Remain(true)))

				}
			}

			select {
			case <-ctx.Done():
				return ctx.Err()

			default:

			}
		}

		return nil
	})

	eg.Go(func() error {
		ticker := time.NewTicker(time.Second)
		resetloggers := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		defer resetloggers.Stop()

		for {
			select {
			case <-ticker.C:
				termbox.Clear(termbox.ColorDefault, termbox.ColorDefault)
				printf(0, 0,
					termbox.ColorWhite,
					termbox.ColorBlack,
					"set size: %.2f,	diff: %.f, limit n/o: %d/%d",
					size,
					diff,
					c.C.Limit.Remain(false), c.C.Limit.Remain(true))
				volS, _, _ := c.SE.Volume()
				volF, _, _ := c.FE.Volume()
				printf(0, 1,
					termbox.ColorWhite,
					termbox.ColorBlack,
					"LTP: %.f / %.f, Spread: %.f / %.f,	volume: %.2f / %.2f",
					c.SE.LTP(), c.FE.LTP(),
					c.SE.Spread(), c.FE.Spread(),
					volS, volF)
				_, _, sum := c.FO.Positions.Sum()
				printf(0, 2,
					termbox.ColorWhite,
					termbox.ColorBlack,
					"has size: %.2f, SFD: %f％,	delay: %.3f / %.3f sec",
					sum,
					(c.SFD.Ratio()-1)*100,
					c.SE.Delay().Seconds(),
					c.FE.Delay().Seconds())

				for i, v := range loggers.Copy() {
					print(0, 10+i, termbox.ColorWhite, termbox.ColorBlack, v)
				}
				termbox.Flush()

			case <-resetloggers.C:
				loggers.Reset()

			case <-ctx.Done():
				return ctx.Err()

			}
		}

	})

	if err := eg.Wait(); err != nil {
		loggers.logs = append(loggers.logs, err.Error())
	}

	f.Close()
	cancel()

}

type Loggers struct {
	sync.RWMutex

	logs  []string
	elogs []string
}

func NewLoggers() *Loggers {
	return &Loggers{
		logs:  make([]string, 0),
		elogs: make([]string, 0),
	}
}

func (p *Loggers) Set(isError bool, s string) {
	p.Lock()
	defer p.Unlock()

	if !isError {
		p.logs = append(p.logs, s)
	}
	p.elogs = append(p.elogs, s)
}

func (p *Loggers) Reset() {
	p.Lock()
	defer p.Unlock()

	p.logs = []string{}
	p.elogs = []string{}
}

func (p *Loggers) Copy() []string {
	p.RLock()
	defer p.RUnlock()

	return append(p.logs, p.elogs...)
}

func printf(x, y int, fg, bg termbox.Attribute, format string, args ...interface{}) {
	s := fmt.Sprintf(format, args...)
	print(x, y, fg, bg, s)
}

func print(x, y int, fg, bg termbox.Attribute, msg string) {
	for _, c := range msg {
		termbox.SetCell(x, 10+y, c, fg, bg)
		x++
	}
}
