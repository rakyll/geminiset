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

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"

	"github.com/rakyll/geminiset/pkg/controller"
	"github.com/rakyll/geminiset/pkg/gemini"
	"github.com/rakyll/geminiset/pkg/ui"
)

func main() {
	var kubeconfig string
	var model string
	var pollInterval time.Duration

	flag.StringVar(&kubeconfig, "kubeconfig", "", "absolute path to the kubeconfig file")
	flag.StringVar(&model, "model", "gemini-3.7-flash", "Gemini model to use")
	flag.DurationVar(&pollInterval, "interval", 3*time.Second, "Reconciliation polling interval")
	flag.Parse()

	ui.PrintBanner()
	log.Println("[GeminiController] Starting GeminiSet Workload Controller...")

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
			log.Fatalf("[GeminiController] Error building kubeconfig: %v", err)
		}
	}

	kubeClientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatalf("[GeminiController] Error creating kubernetes clientset: %v", err)
	}

	dynClient, err := dynamic.NewForConfig(config)
	if err != nil {
		log.Fatalf("[GeminiController] Error creating dynamic client: %v", err)
	}

	geminiClient := controller.NewDynamicGeminiSetClient(dynClient)
	engine, err := gemini.NewClient("", model)
	if err != nil {
		log.Fatalf("[GeminiController] Failed to initialize Gemini engine: %v", err)
	}

	ctrl := controller.NewController(kubeClientset, geminiClient, engine)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go ctrl.Start(ctx, pollInterval)

	<-sigCh
	log.Println("[GeminiController] Shutting down controller...")
	ctrl.Stop()
}
