// Example function-based Apache Kafka producer with a custom delivery channel
package main

/**
 * Copyright 2016 Confluent Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

import (
	"fmt"
	"os"

	"github.com/confluentinc/confluent-kafka-go/kafka"
)

func main() {

	if len(os.Args) != 3 {
		fmt.Fprintf(os.Stderr, "Usage: %s <broker> <topic>\n",
			os.Args[0])
		os.Exit(1)
	}

	broker := os.Args[1]
	topic := os.Args[2]
	totalMsgcnt := 3

	p, err := kafka.NewProducer(&kafka.ConfigMap{"bootstrap.servers": broker})

	if err != nil {
		fmt.Printf("Failed to create producer: %s\n", err)
		os.Exit(1)
	}

	fmt.Printf("Created Producer %v\n", p)

	// Listen to all the client instance-level errors.
	// It's important to read these errors too otherwise the events channel will eventually fill up
	doneEventsChan := make(chan bool)
	go func() {
		defer close(doneEventsChan)
		for e := range p.Events() {
			switch ev := e.(type) {
			case kafka.Error:
				// Generic client instance-level errors, such as
				// broker connection failures, authentication issues, etc.
				//
				// These errors should generally be considered informational
				// as the underlying client will automatically try to
				// recover from any errors encountered, the application
				// does not need to take action on them.
				fmt.Printf("Error: %v\n", ev)
			default:
				fmt.Printf("Ignored event: %s\n", ev)
			}
		}
	}()

	// Optional delivery channel, if not specified the Producer object's
	// .Events channel is used.
	deliveryChan := make(chan kafka.Event)
	doneDeliveryChan := make(chan bool)
	producerQueueFree := make(chan bool)
	go func() {
		msgcnt := 0
		defer close(doneDeliveryChan)
		defer close(producerQueueFree)
		for e := range deliveryChan {
			switch ev := e.(type) {
			case *kafka.Message:
				m := ev
				if m.TopicPartition.Error != nil {
					fmt.Printf("Delivery failed: %v\n", m.TopicPartition.Error)
				} else {
					fmt.Printf("Delivered message to topic %s [%d] at offset %v\n",
						*m.TopicPartition.Topic, m.TopicPartition.Partition, m.TopicPartition.Offset)
				}
				// Signals (non-blocking) that the producer queue has been freed
				select {
				case producerQueueFree <- true:
				default:
				}
				msgcnt++
				if msgcnt == totalMsgcnt {
					return
				}

			default:
				fmt.Printf("Ignored event: %s\n", ev)
			}
		}
	}()

	msgcnt := 0
	for msgcnt < totalMsgcnt {
		value := fmt.Sprintf("Producer example, message #%d", msgcnt)

		err = p.Produce(&kafka.Message{
			TopicPartition: kafka.TopicPartition{Topic: &topic, Partition: kafka.PartitionAny},
			Value:          []byte(value),
			Headers:        []kafka.Header{{Key: "myTestHeader", Value: []byte("header values are binary")}},
		}, deliveryChan)

		if err != nil {
			if err.(kafka.Error).Code() == kafka.ErrQueueFull {
				// Producer queue is full, waits for the it to be freed
				_ = <-producerQueueFree
				continue
			}
			fmt.Printf("Failed to produce message: %v\n", err)
			// Close the custom delivery channel as no delivery messages will be sent
			close(deliveryChan)
			// Close the producer and the events channel
			p.Close()
			// Wait for the goroutine receiving client errors
			_ = <-doneEventsChan
			os.Exit(1)
		}
		msgcnt++
	}

	// Wait for delivery report goroutine with custom channel to finish
	_ = <-doneDeliveryChan

	// Close the custom delivery channel as all the delivery report messages have been sent
	close(deliveryChan)

	// Close the producer and the events channel
	p.Close()

	// Wait for the goroutine receiving client errors
	_ = <-doneEventsChan
}
