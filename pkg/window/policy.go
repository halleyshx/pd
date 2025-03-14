// The MIT License (MIT)
// Copyright (c) 2022 go-kratos Project Authors.
//
// Copyright 2023 TiKV Project Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,g
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package window

import (
	"sync"
	"time"
)

// RollingPolicy is a policy for ring window based on time duration.
// RollingPolicy moves bucket offset with time duration.
// e.g. If the last point is appended one bucket duration ago,
// RollingPolicy will increment current offset.
type RollingPolicy struct {
	mu     sync.RWMutex
	size   int
	window *Window
	offset int

	bucketDuration time.Duration
	lastAppendTime time.Time
}

// RollingPolicyOpts contains the arguments for creating RollingPolicy.
type RollingPolicyOpts struct {
	BucketDuration time.Duration
}

// NewRollingPolicy creates a new RollingPolicy based on the given window and RollingPolicyOpts.
func NewRollingPolicy(window *Window, opts RollingPolicyOpts) *RollingPolicy {
	return &RollingPolicy{
		window: window,
		size:   window.Size(),
		offset: 0,

		bucketDuration: opts.BucketDuration,
		lastAppendTime: time.Now(),
	}
}

// timespan returns passed bucket number since lastAppendTime,
// if it is one bucket duration earlier than the last recorded
// time, it will return the size.
func (r *RollingPolicy) timespan() int {
	v := int(time.Since(r.lastAppendTime) / r.bucketDuration)
	if v > -1 { // maybe time backwards
		return v
	}
	return r.size
}

// apply applies function f with value val on
// current offset bucket, expired bucket will be reset
func (r *RollingPolicy) apply(f func(offset int, val float64), val float64) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// calculate current offset
	timespan := r.timespan()
	oriTimespan := timespan
	if timespan > 0 {
		start := (r.offset + 1) % r.size
		end := (r.offset + timespan) % r.size
		if timespan > r.size {
			timespan = r.size
		}
		// reset the expired buckets
		r.window.ResetBuckets(start, timespan)
		r.offset = end
		r.lastAppendTime = r.lastAppendTime.Add(time.Duration(oriTimespan * int(r.bucketDuration)))
	}
	f(r.offset, val)
}

// Add adds the given value to the latest point within bucket.
func (r *RollingPolicy) Add(val float64) {
	r.apply(r.window.Add, val)
}

// Reduce applies the reduction function to all buckets within the window.
func (r *RollingPolicy) Reduce(f func(Iterator) float64) (val float64) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	timespan := r.timespan()
	if count := r.size - timespan; count > 0 {
		offset := r.offset + timespan + 1
		if offset >= r.size {
			offset -= r.size
		}
		val = f(r.window.Iterator(offset, count))
	}
	return val
}
