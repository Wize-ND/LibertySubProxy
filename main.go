// Package main — лёгкий реверс-прокси для роутера Keenetic Hopper SE (aarch64).
// GET /roskom-whocares/{sub} -> https://connliberty.com/connection/subs/{sub}.
// Поддерживает демонизацию (-d) под Entware/opkg init-скриптом.
package main

import (
	"context"
	"flag"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"
)

const defaultUpstream = "https://connliberty.com/connection/subs/"

// downstreamClient — тонко настроенный транспорт: минимум горутин и памяти.
var downstreamClient *http.Client

// buildClient собирает http.Client с урезанным транспортным пулом.
func buildClient() *http.Client {
	transport := &http.Transport{
		// Пулы соединений минимальные: роутер, 1-2 клиента одновременно.
		MaxIdleConns:        2,
		MaxIdleConnsPerHost: 2,
		MaxConnsPerHost:     4,
		IdleConnTimeout:     30 * time.Second,
		// Быстрее освобождаем ресурсы.
		ResponseHeaderTimeout: 15 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		DisableKeepAlives:     false,
		ForceAttemptHTTP2:     false, // HTTP/2 не нужен — меньше памяти
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
	}
	return &http.Client{
		Timeout: 30 * time.Second,
		// Редиректы не следуем — просто транслируем ответ как есть.
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Transport: transport,
	}
}

// proxyHandler копирует входящий запрос на upstream и транслирует ответ.
func proxyHandler(w http.ResponseWriter, r *http.Request, upstream string) {
	sub := strings.TrimPrefix(r.URL.Path, "/roskom-whocares/")
	if sub == "" || strings.Contains(sub, "/") {
		http.Error(w, "bad path, expected /roskom-whocares/{sub}", http.StatusBadRequest)
		return
	}

	target, err := url.Parse(upstream + url.PathEscape(sub))
	if err != nil {
		http.Error(w, "bad upstream url: "+err.Error(), http.StatusInternalServerError)
		return
	}

	outReq, err := http.NewRequestWithContext(r.Context(), r.Method, target.String(), r.Body)
	if err != nil {
		http.Error(w, "request build failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	for key, vals := range r.Header {
		for _, v := range vals {
			outReq.Header.Add(key, v)
		}
	}
	outReq.Header.Set("X-Forwarded-For", r.RemoteAddr)
	outReq.Header.Set("X-Forwarded-Host", r.Host)
	outReq.Header.Set("X-Forwarded-Proto", "http")

	resp, err := downstreamClient.Do(outReq)
	if err != nil {
		log.Printf("upstream error sub=%q: %v", sub, err)
		http.Error(w, "upstream failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	for key, vals := range resp.Header {
		for _, v := range vals {
			w.Header().Add(key, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	if _, err := io.Copy(w, resp.Body); err != nil {
		log.Printf("copy body sub=%q: %v", sub, err)
	}
}

func main() {
	var (
		listen   = flag.String("listen", ":65432", "адрес прослушивания host:port")
		upstream = flag.String("upstream", defaultUpstream, "базовый URL upstream")
		daemon   = flag.Bool("d", false, "демонизироваться (отцепиться от терминала)")
		pidFile  = flag.String("pidfile", "/opt/var/run/libertysubproxy.pid", "путь к pid-файлу")
		memLimit = flag.Int64("memlimit", 16, "soft memory limit Go GC в MiB (GOMEMLIMIT)")
		ver      = flag.Bool("version", false, "печатать версию и выйти")
	)
	flag.Parse()

	if *ver {
		println("libertysubproxy 1.0.0")
		return
	}

	// --- Минимизация потребления памяти ---
	// GOGC=50: чаще сборка, меньше пик RSS — критично на роутере с 256-512 МБ.
	_ = os.Setenv("GOGC", "50")
	// GOMEMLIMIT: жёсткий софт-лимит кучи, GC не даст переесть память роутера.
	_ = os.Setenv("GOMEMLIMIT", itoa64(*memLimit)+"MiB")
	// Ограничиваем число P — меньше служебных структур планировщика.
	if runtime.NumCPU() > 2 {
		runtime.GOMAXPROCS(2)
	}

	// --- Демонизация (только Linux) ---
	if *daemon {
		child, err := daemonize(*pidFile)
		if err != nil {
			log.Fatalf("daemonize: %v", err)
		}
		if child { // это дочерний процесс — продолжаем работу демоном
			goto started
		}
		return // родитель — выходим, демон уже работает
	}

started:
	if *daemon {
		_ = writePID(*pidFile)
	}

	downstreamClient = buildClient()

	mux := http.NewServeMux()
	mux.HandleFunc("/roskom-whocares/", func(w http.ResponseWriter, r *http.Request) {
		proxyHandler(w, r, *upstream)
	})
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	srv := &http.Server{
		Addr:              *listen,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// Graceful shutdown по SIGTERM/SIGINT (init-скрипт шлёт SIGTERM).
	done := make(chan struct{})
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
		<-sig
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
		close(done)
	}()

	log.Printf("libertysubproxy: listen=%s upstream=%s memlimit=%dMiB daemon=%v",
		*listen, *upstream, *memLimit, *daemon)

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server: %v", err)
	}
	<-done
	if *daemon {
		_ = os.Remove(*pidFile)
	}
}

// itoa64 — маленькая утилита без strconv-импорта здесь не нужна, но экономнее так.
func itoa64(v int64) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
