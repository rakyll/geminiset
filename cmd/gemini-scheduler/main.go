package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"

	"github.com/rakyll/geminiset/pkg/gemini"
	"github.com/rakyll/geminiset/pkg/scheduler"
	"github.com/rakyll/geminiset/pkg/ui"
)

func main() {
	var kubeconfig string
	var model string
	var pollInterval time.Duration

	flag.StringVar(&kubeconfig, "kubeconfig", "", "absolute path to the kubeconfig file")
	flag.StringVar(&model, "model", "gemini-3.7-flash", "Gemini model to use")
	flag.DurationVar(&pollInterval, "interval", 2*time.Second, "Polling interval for scheduling queue")
	flag.Parse()

	ui.PrintBanner()
	log.Println("[GeminiScheduler] Initializing Gemini Flash 3.7 AI Scheduler for Kubernetes...")

	// Resolve kubeconfig
	var config *rest.Config
	var err error
	if kubeconfig == "" {
		if home := homedir.HomeDir(); home != "" {
			kubeconfig = filepath.Join(home, ".kube", "config")
		}
	}

	config, err = rest.InClusterConfig()
	if err != nil {
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			log.Fatalf("[GeminiScheduler] Error building kubeconfig: %v", err)
		}
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatalf("[GeminiScheduler] Error building kubernetes clientset: %v", err)
	}

	engine, err := gemini.NewClient("", model)
	if err != nil {
		log.Fatalf("[GeminiScheduler] Failed to initialize Gemini engine: %v", err)
	}
	log.Printf("[GeminiScheduler] Engine active: %s", engine.Model())

	sched := scheduler.NewScheduler(clientset, engine)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go sched.Start(ctx, pollInterval)

	<-sigCh
	log.Println("[GeminiScheduler] Shutting down scheduler...")
	sched.Stop()
}
