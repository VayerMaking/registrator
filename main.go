package main

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/mux"
)

type Request struct {
	Data       url.Values
	ScheduleAt time.Time
}

type Scheduler struct {
	Requests []Request
	Lock     sync.Mutex
}

func main() {
	scheduler := Scheduler{}
	var wg sync.WaitGroup

	// Start a goroutine to process the scheduled requests
	wg.Add(1)
	go scheduler.processRequests(&wg)

	// Start an HTTP server to receive data
	r := mux.NewRouter()
	r.HandleFunc("/schedule-request", scheduler.scheduleRequest).Methods("POST")

	log.Fatal(http.ListenAndServe(":8080", r))
	wg.Wait()
}

func (s *Scheduler) scheduleRequest(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		log.Println("Failed to parse form:", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Extract data from the request
	data := r.PostForm
	scheduleTime := r.FormValue("schedule_time")

	// Parse the schedule time with the Netherlands timezone
	layout := "2006-01-02 15:04:05"
	loc, err := time.LoadLocation("Europe/Amsterdam")
	if err != nil {
		log.Println("Failed to load location:", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	scheduleAt, err := time.ParseInLocation(layout, scheduleTime, loc)
	if err != nil {
		log.Println("Failed to parse schedule time:", err)
		http.Error(w, "Invalid schedule time", http.StatusBadRequest)
		return
	}

	// Create a new request to schedule
	newReq := Request{
		Data:       data,
		ScheduleAt: scheduleAt,
	}

	// Add the new request to the scheduler
	s.Lock.Lock()
	s.Requests = append(s.Requests, newReq)
	s.Lock.Unlock()

	fmt.Fprintln(w, "Request scheduled successfully")
}

func (s *Scheduler) processRequests(wg *sync.WaitGroup) {
	defer wg.Done()

	for {
		// Sleep for a short duration to avoid tight loop
		time.Sleep(1 * time.Second)

		// Get the current time in the Netherlands timezone
		loc, err := time.LoadLocation("Europe/Amsterdam")
		if err != nil {
			log.Println("Failed to load location:", err)
			continue
		}
		now := time.Now().In(loc)

		// Check if there are any requests to be sent
		s.Lock.Lock()
		requests := make([]Request, 0, len(s.Requests))
		for _, req := range s.Requests {
			if now.After(req.ScheduleAt) || now.Equal(req.ScheduleAt) {
				// Prepare the HTTP request
				endpoint := "https://magisrent.nl/interestlist"
				body := bytes.NewBufferString(req.Data.Encode())
				httpReq, err := http.NewRequest("POST", endpoint, body)
				if err != nil {
					log.Println("Failed to create request:", err)
					continue
				}

				// Set the required headers
				httpReq.Header.Set("Referer", "https://magisrent.nl/interestlist?building=MRX")
				httpReq.Header.Set("Origin", "https://magisrent.nl")
				httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				httpReq.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
				httpReq.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/16.3 Safari/605.1.15")

				// Send the HTTP request
				client := &http.Client{}
				resp, err := client.Do(httpReq)
				if err != nil {
					log.Println("Request failed:", err)
					continue
				}
				defer resp.Body.Close()

				// Handle the response as needed
				log.Println("Request sent successfully")
			} else {
				// Add the request to the updated list
				requests = append(requests, req)
			}
		}
		// Update the list of requests
		s.Requests = requests
		s.Lock.Unlock()
	}
}
