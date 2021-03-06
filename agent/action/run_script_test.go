package action_test

import (
	. "github.com/cloudfoundry/bosh-agent/internal/github.com/onsi/ginkgo"
	. "github.com/cloudfoundry/bosh-agent/internal/github.com/onsi/gomega"

	"github.com/cloudfoundry/bosh-agent/agent/action"
	"github.com/cloudfoundry/bosh-agent/agent/applier/applyspec"
	fakeapplyspec "github.com/cloudfoundry/bosh-agent/agent/applier/applyspec/fakes"
	"github.com/cloudfoundry/bosh-agent/agent/scriptrunner"
	fakescript "github.com/cloudfoundry/bosh-agent/agent/scriptrunner/fakes"
	"github.com/cloudfoundry/bosh-agent/internal/github.com/cloudfoundry/bosh-utils/logger"
	"time"
)

var _ = Describe("RunScript", func() {
	var (
		runScriptAction       action.RunScriptAction
		fakeJobScriptProvider *fakescript.FakeJobScriptProvider
		specService           *fakeapplyspec.FakeV1Service
		log                   logger.Logger
		options               map[string]interface{}
		scriptName            string
	)

	createFakeJob := func(jobName string) {
		specService.Spec.JobSpec.JobTemplateSpecs = append(specService.Spec.JobSpec.JobTemplateSpecs, applyspec.JobTemplateSpec{Name: jobName})
	}

	BeforeEach(func() {
		log = logger.NewLogger(logger.LevelNone)
		fakeJobScriptProvider = &fakescript.FakeJobScriptProvider{}
		specService = fakeapplyspec.NewFakeV1Service()
		createFakeJob("fake-job-1")
		runScriptAction = action.NewRunScript(fakeJobScriptProvider, specService, log)
		scriptName = "run-me"
		options = make(map[string]interface{})
	})

	It("is asynchronous", func() {
		Expect(runScriptAction.IsAsynchronous()).To(BeTrue())
	})

	It("is not persistent", func() {
		Expect(runScriptAction.IsPersistent()).To(BeFalse())
	})

	Context("when script exists", func() {

		var existingScript *fakescript.FakeScript

		BeforeEach(func() {
			existingScript = &fakescript.FakeScript{}
			existingScript.ExistsReturns(true)
			fakeJobScriptProvider.GetReturns(existingScript)
		})

		It("is executed", func() {
			existingScript.RunStub = func(errorChan chan scriptrunner.RunScriptResult, doneChan chan scriptrunner.RunScriptResult) {
				doneChan <- scriptrunner.RunScriptResult{JobName: "fake-job-1", ScriptPath: "path/to/script"}
			}
			results, err := runScriptAction.Run(scriptName, options)

			Expect(err).ToNot(HaveOccurred())
			Expect(results).To(Equal(map[string]string{"fake-job-1": "executed"}))
		})

		It("gives an error when script fails", func() {
			existingScript.RunStub = func(errorChan chan scriptrunner.RunScriptResult, doneChan chan scriptrunner.RunScriptResult) {
				errorChan <- scriptrunner.RunScriptResult{JobName: "fake-job-1", ScriptPath: "path/to/script"}
			}

			results, err := runScriptAction.Run(scriptName, options)

			Expect(err).To(HaveOccurred())
			Expect(results).To(Equal(map[string]string{"fake-job-1": "failed"}))
		})

	})

	Context("when running scripts concurrently", func() {

		var existingScript *fakescript.FakeScript
		var existingScript2 *fakescript.FakeScript

		BeforeEach(func() {
			existingScript = &fakescript.FakeScript{}
			existingScript.ExistsReturns(true)

			createFakeJob("fake-job-2")
			existingScript2 = &fakescript.FakeScript{}
			existingScript2.ExistsReturns(true)

			fakeJobScriptProvider.GetStub = func(jobName string, relativePath string) scriptrunner.Script {
				if jobName == "fake-job-1" {
					return existingScript
				} else if jobName == "fake-job-2" {
					return existingScript2
				}
				return nil
			}
		})

		It("is executed and both scripts pass", func() {
			existingScript.RunStub = func(errorChan chan scriptrunner.RunScriptResult, doneChan chan scriptrunner.RunScriptResult) {
				doneChan <- scriptrunner.RunScriptResult{JobName: "fake-job-1", ScriptPath: "path/to/script"}
			}
			existingScript2.RunStub = func(errorChan chan scriptrunner.RunScriptResult, doneChan chan scriptrunner.RunScriptResult) {
				doneChan <- scriptrunner.RunScriptResult{JobName: "fake-job-2", ScriptPath: "path/to/script2"}
			}

			results, err := runScriptAction.Run(scriptName, options)

			Expect(err).ToNot(HaveOccurred())
			Expect(results).To(Equal(map[string]string{"fake-job-1": "executed", "fake-job-2": "executed"}))
		})

		It("returns two failed statuses when both scripts fail", func() {
			existingScript.RunStub = func(errorChan chan scriptrunner.RunScriptResult, doneChan chan scriptrunner.RunScriptResult) {
				errorChan <- scriptrunner.RunScriptResult{JobName: "fake-job-1", ScriptPath: "path/to/script"}
			}
			existingScript2.RunStub = func(errorChan chan scriptrunner.RunScriptResult, doneChan chan scriptrunner.RunScriptResult) {
				errorChan <- scriptrunner.RunScriptResult{JobName: "fake-job-2", ScriptPath: "path/to/script2"}
			}

			results, err := runScriptAction.Run(scriptName, options)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).Should(ContainSubstring("2 of 2 run-me scripts failed. Failed Jobs:"))
			Expect(err.Error()).Should(ContainSubstring("fake-job-1"))
			Expect(err.Error()).Should(ContainSubstring("fake-job-2"))
			Expect(err.Error()).ShouldNot(ContainSubstring("Successful Jobs"))

			Expect(results).To(Equal(map[string]string{"fake-job-1": "failed", "fake-job-2": "failed"}))
		})

		It("returns one failed status when first script fail and second script pass, and when one fails continue waiting for unfinished tasks", func() {
			existingScript.RunStub = func(errorChan chan scriptrunner.RunScriptResult, doneChan chan scriptrunner.RunScriptResult) {
				time.Sleep(2 * time.Second)
				errorChan <- scriptrunner.RunScriptResult{JobName: "fake-job-1", ScriptPath: "path/to/script"}
			}
			existingScript2.RunStub = func(errorChan chan scriptrunner.RunScriptResult, doneChan chan scriptrunner.RunScriptResult) {
				doneChan <- scriptrunner.RunScriptResult{JobName: "fake-job-2", ScriptPath: "path/to/script2"}
			}

			results, err := runScriptAction.Run(scriptName, options)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("1 of 2 run-me scripts failed. Failed Jobs: fake-job-1. Successful Jobs: fake-job-2."))
			Expect(results).To(Equal(map[string]string{"fake-job-1": "failed", "fake-job-2": "executed"}))
		})

		It("returns one failed status when first script pass and second script fail", func() {
			existingScript.RunStub = func(errorChan chan scriptrunner.RunScriptResult, doneChan chan scriptrunner.RunScriptResult) {
				doneChan <- scriptrunner.RunScriptResult{JobName: "fake-job-1", ScriptPath: "path/to/script"}
			}
			existingScript2.RunStub = func(errorChan chan scriptrunner.RunScriptResult, doneChan chan scriptrunner.RunScriptResult) {
				errorChan <- scriptrunner.RunScriptResult{JobName: "fake-job-2", ScriptPath: "path/to/script2"}
			}

			results, err := runScriptAction.Run(scriptName, options)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("1 of 2 run-me scripts failed. Failed Jobs: fake-job-2. Successful Jobs: fake-job-1."))
			Expect(results).To(Equal(map[string]string{"fake-job-1": "executed", "fake-job-2": "failed"}))
		})

		It("wait for scripts to finish", func() {
			existingScript.RunStub = func(errorChan chan scriptrunner.RunScriptResult, doneChan chan scriptrunner.RunScriptResult) {
				time.Sleep(2 * time.Second)
				doneChan <- scriptrunner.RunScriptResult{JobName: "fake-job-1", ScriptPath: "path/to/script"}
			}
			existingScript2.RunStub = func(errorChan chan scriptrunner.RunScriptResult, doneChan chan scriptrunner.RunScriptResult) {
				doneChan <- scriptrunner.RunScriptResult{JobName: "fake-job-2", ScriptPath: "path/to/script2"}
			}

			results, err := runScriptAction.Run(scriptName, options)

			Expect(err).ToNot(HaveOccurred())
			Expect(results).To(Equal(map[string]string{"fake-job-1": "executed", "fake-job-2": "executed"}))
		})

	})

	Context("when script does not exist", func() {

		var nonExistingScript *fakescript.FakeScript

		BeforeEach(func() {
			nonExistingScript = &fakescript.FakeScript{}
			nonExistingScript.ExistsReturns(false)
			fakeJobScriptProvider.GetReturns(nonExistingScript)
		})

		It("does not return a status for that script", func() {
			results, err := runScriptAction.Run(scriptName, options)

			Expect(err).ToNot(HaveOccurred())
			Expect(results).To(Equal(map[string]string{}))
		})
	})
})
