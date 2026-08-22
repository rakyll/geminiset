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
	"github.com/rakyll/geminiset/pkg/scheduler"
	"github.com/rakyll/geminiset/pkg/ui"
)

func main() {
	var kubeconfig string
	var model string
	var ctrlInterval time.Duration
	var schedInterval time.Duration

	flag.StringVar(&kubeconfig, "kubeconfig", "", "absolute path to the kubeconfig file")
	flag.StringVar(&model, "model", "gemini-3.7-flash", "Gemini model to use")
	flag.DurationVar(&ctrlInterval, "controller-interval", 3*time.Second, "Controller polling interval")
	flag.DurationVar(&schedInterval, "scheduler-interval", 2*time.Second, "Scheduler polling interval")
	flag.Parse()

	ui.PrintBanner()
	log.Println("[GeminiOperator] Starting Unified Gemini Kubernetes Operator (Controller + Scheduler)...")

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
			log.Fatalf("[GeminiOperator] Error building kubeconfig: %v", err)
		}
	}

	kubeClientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatalf("[GeminiOperator] Error creating kubernetes clientset: %v", err)
	}

	dynClient, err := dynamic.NewForConfig(config)
	if err != nil {
		log.Fatalf("[GeminiOperator] Error creating dynamic client: %v", err)
	}

	geminiClient := controller.NewDynamicGeminiSetClient(dynClient)
	engine, err := gemini.NewClient("", model)
	if err != nil {
		log.Fatalf("[GeminiOperator] Failed to initialize Gemini engine: %v", err)
	}
	log.Printf("[GeminiOperator] AI Engine: %s", engine.Model())

	ctrl := controller.NewController(kubeClientset, geminiClient, engine)
	sched := scheduler.NewScheduler(kubeClientset, engine)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go ctrl.Start(ctx, ctrlInterval)
	go sched.Start(ctx, schedInterval)

	<-sigCh
	log.Println("[GeminiOperator] Shutting down operator...")
	ctrl.Stop()
	sched.Stop()
}
