// Copyright 2015 Alex Goussiatiner. All rights reserved.
// Use of this source code is governed by a MIT
// license that can be found in the LICENSE file.
//
// Godes  is the general-purpose simulation library
// which includes the  simulation engine  and building blocks
// for modeling a wide variety of systems at varying levels of details.
//
// Godes model controls the runnners
// See examples for the usage.
//

package godes

import (
	"container/list"
	"fmt"
	//"sync"
	"time"
)

const simulationSecondScale = 100
const rUNNER_STATE_READY = 0
const rUNNER_STATE_ACTIVE = 1
const rUNNER_STATE_WAITING_COND = 2
const rUNNER_STATE_SCHEDULED = 3
const rUNNER_STATE_INTERRUPTED = 4
const rUNNER_STATE_TERMINATED = 5

var modl *model
var stime float64 = 0

// WaitUntilDone stops the main goroutine and waits 
// until all the runners finished executing the Run()
func WaitUntilDone() {
	if modl == nil {
		panic(" not initilized")
	}
	modl.waitUntillDone()
}

//AddRunner adds the runner into model and 
func AddRunner(runner RunnerInterface) {
	if runner == nil {
		panic("runner is nil")
	}
	if modl == nil {
		createModel(false)
	}
	modl.add(runner)
}

//Interrupt holds the runner execution
func Interrupt(runner RunnerInterface) {
	if runner == nil {
		panic("runner is nil")
	}
	if modl == nil {
		panic("model is nil")
	}
	modl.interrupt(runner)
}

//Resume restarts the runner execution
func Resume(runner RunnerInterface, timeChange float64) {
	if runner == nil {
		panic("runner is nil")
	}
	if modl == nil {
		panic("model is nil")
	}
	modl.resume(runner, timeChange)
}

//Run starts the simulation model.
// Must be called explicitly.
func Run() {
	if modl == nil {
		createModel(false)
	}
	//assuming that it comes from the main go routine
	if modl.activeRunner == nil {
		panic("runner is nil")
	}

	if modl.activeRunner.getInternalId() != 0 {
		panic("it comes from not from the main go routine")
	}

	modl.simulationActive = true
	modl.control()

}

//Advance the simulation time
func Advance(interval float64) {
	if modl == nil {
		createModel(false)
	}
	modl.advance(interval)
}

// Verbose sets the model in the verbose mode
func Verbose(v bool) {
	if modl == nil {
		createModel(v)
	}
	modl.DEBUG = v
}
// Clear the model between the runs
func Clear() {
	if modl == nil {
		panic(" No model exist")
	} else {

		stime = 0
		modl = newModel(modl.DEBUG)
		//model.simulationActive = true
		//model.control()
	}
}

// GetSystemTime retuns the current simulation time
func GetSystemTime() float64 {
	return stime
}
// GetSystemTime retuns the current simulation time
func Yield()  {
	Advance(0.01)
}



// createModel
func createModel(verbose bool) {
	if modl != nil {
		panic("model is already active")
	}
	stime = 0
	modl = newModel(verbose)
	//model.simulationActive = true
	//model.control()
	//assuming that it comes from the main go routine
}



type model struct {
	//mu                  sync.RWMutex
	activeRunner        RunnerInterface
	movingList          *list.List
	scheduledList       *list.List
	waitingList         *list.List
	waitingConditionMap map[int]RunnerInterface
	interruptedMap      map[int]RunnerInterface
	terminatedList      *list.List
	currentId           int
	controlChannel      chan int
	simulationActive    bool
	DEBUG               bool
}

//newModel initilizes the model
func newModel(verbose bool) *model {

	var ball *Runner = NewRunner()
	ball.channel = make(chan int)
	ball.markTime = time.Now()
	ball.internalId = 0
	ball.state = rUNNER_STATE_ACTIVE //that is bypassing READY
	ball.priority = 100
	ball.setMarkTime(time.Now())
	var runner RunnerInterface = ball
	mdl := model{activeRunner: runner, controlChannel: make(chan int), DEBUG: verbose, simulationActive: false}
	mdl.addToMovingList(runner)
	return &mdl
}

func (mdl *model) advance(interval float64) bool {

	ch := mdl.activeRunner.getChannel()
	mdl.activeRunner.setMovingTime(stime + interval)
	mdl.activeRunner.setState(rUNNER_STATE_SCHEDULED)
	mdl.removeFromMovingList(mdl.activeRunner)
	mdl.addToSchedulledList(mdl.activeRunner)
	//restart control channel and freez
	mdl.controlChannel <- 100
	<-ch
	return true
}

func (mdl *model) waitUntillDone() {

	if mdl.activeRunner.getInternalId() != 0 {
		panic("waitUntillDone initiated for not main ball")
	}

	mdl.removeFromMovingList(mdl.activeRunner)
	mdl.controlChannel <- 100
	for {

		if !modl.simulationActive {
			break
		} else {
			if mdl.DEBUG {
				fmt.Println("waiting", mdl.movingList.Len())
			}
			time.Sleep(time.Millisecond * simulationSecondScale)
		}
	}
}

func (mdl *model) add(runner RunnerInterface) bool {

	mdl.currentId++
	runner.setChannel(make(chan int))
	runner.setMovingTime(stime)
	runner.setInternalId(mdl.currentId)
	runner.setState(rUNNER_STATE_READY)
	mdl.addToMovingList(runner)

	go func() {
		<-runner.getChannel()
		runner.setMarkTime(time.Now())
		runner.Run()
		if mdl.activeRunner == nil {
			panic("remove: activeRunner == nil")
		}
		mdl.removeFromMovingList(mdl.activeRunner)
		mdl.activeRunner.setState(rUNNER_STATE_TERMINATED)
		mdl.activeRunner = nil
		mdl.controlChannel <- 100
	}()
	return true

}

func (mdl *model) interrupt(runner RunnerInterface) {

	if runner.getState() != rUNNER_STATE_SCHEDULED {
		panic("It is not  rUNNER_STATE_SCHEDULED")
	}
	mdl.removeFromSchedulledList(runner)
	runner.setState(rUNNER_STATE_INTERRUPTED)
	mdl.addToInterruptedMap(runner)

}

func (mdl *model) resume(runner RunnerInterface, timeChange float64) {
	if runner.getState() != rUNNER_STATE_INTERRUPTED {
		panic("It is not  rUNNER_STATE_INTERRUPTED")
	}
	mdl.removeFromInterruptedMap(runner)
	runner.setState(rUNNER_STATE_SCHEDULED)
	runner.setMovingTime(runner.getMovingTime() + timeChange)
	mdl.addToMovingList(runner)

}

func (mdl *model) booleanControlWait(b *BooleanControl, val bool) {

	ch := mdl.activeRunner.getChannel()
	if mdl.activeRunner == nil {
		panic("booleanControlWait - no runner")
	}

	mdl.removeFromMovingList(mdl.activeRunner)

	mdl.activeRunner.setState(rUNNER_STATE_WAITING_COND)
	mdl.activeRunner.setWaitingForBool(val)
	mdl.activeRunner.setWaitingForBoolControl(b)

	mdl.addToWaitingConditionMap(mdl.activeRunner)
	mdl.controlChannel <- 100
	<-ch

}

func (mdl *model) booleanControlWaitAndTimeout(b *BooleanControl, val bool, timeout float64) {

	ri := &TimeoutRunner{&Runner{}, mdl.activeRunner, timeout}
	AddRunner(ri)
	mdl.activeRunner.setWaitingForBoolControlTimeoutId(ri.getInternalId())
	mdl.booleanControlWait(b, val)

}

func (mdl *model) booleanControlSet(b *BooleanControl) {
	ch := mdl.activeRunner.getChannel()
	if mdl.activeRunner == nil {
		panic("booleanControlSet - no runner")
	}
	mdl.controlChannel <- 100
	<-ch

}

func (mdl *model) control() bool {

	if mdl.activeRunner == nil {
		panic("control: activeBall == nil")
	}

	go func() {
		var runner RunnerInterface
		for {
			<-mdl.controlChannel
			if mdl.waitingConditionMap != nil && len(mdl.waitingConditionMap) > 0 {
				for key, temp := range mdl.waitingConditionMap {
					if temp.getWaitingForBoolControl() == nil {
						panic("  no BoolControl")
					}
					if temp.getWaitingForBool() == temp.getWaitingForBoolControl().GetState() {
						temp.setState(rUNNER_STATE_READY)
						temp.setWaitingForBoolControl(nil)
						temp.setWaitingForBoolControlTimeoutId(-1)
						mdl.addToMovingList(temp)
						delete(mdl.waitingConditionMap, key)
						break
					}
				}
			}

			//finding new runner
			runner = nil
			if mdl.movingList != nil && mdl.movingList.Len() > 0 {
				runner = mdl.getFromMovingList()
			}
			if runner == nil && mdl.scheduledList != nil && mdl.scheduledList.Len() > 0 {
				runner = mdl.getFromSchedulledList()
				if runner.getMovingTime() < stime {
					panic("control is seting simulation time in the past")
				} else {
					stime = runner.getMovingTime()
				}
				mdl.addToMovingList(runner)
			}
			if runner == nil {
				break
			}
			//restarting
			mdl.activeRunner = runner
			mdl.activeRunner.setState(rUNNER_STATE_ACTIVE)
			runner.setWaitingForBoolControl(nil)
			mdl.activeRunner.getChannel() <- -1

		}
		if mdl.DEBUG {
			fmt.Println("Finished")
		}
		mdl.simulationActive = false
	}()

	return true

}
