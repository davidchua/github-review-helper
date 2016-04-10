package main_test

import (
	"errors"
	"net/http"
	"net/http/httptest"

	"github.com/google/go-github/github"
	. "github.com/salemove/github-review-helper"
	"github.com/stretchr/testify/mock"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = TestWebhookHandler(func(context WebhookTestContext) {
	Describe("!squash comment", func() {
		var (
			handle      = context.Handle
			headers     = context.Headers
			requestJSON = context.RequestJSON

			responseRecorder *httptest.ResponseRecorder
			pullRequests     *MockPullRequests
		)
		BeforeEach(func() {
			responseRecorder = *context.ResponseRecorder
			pullRequests = *context.PullRequests
		})

		headers.Is(func() map[string]string {
			return map[string]string{
				"X-Github-Event": "issue_comment",
			}
		})
		requestJSON.Is(func() string {
			return IssueCommentEvent("!squash")
		})

		Context("with GitHub request failing", func() {
			BeforeEach(func() {
				pullRequests.
					On("Get", repositoryOwner, repositoryName, issueNumber).
					Return(nil, nil, errors.New("an error"))
			})

			It("fails with a gateway error", func() {
				handle()
				Expect(responseRecorder.Code).To(Equal(http.StatusBadGateway))
			})
		})

		Context("with GitHub request succeeding", func() {
			pr := &github.PullRequest{
				Base: &github.PullRequestBranch{
					SHA:  github.String("1234"),
					Ref:  github.String("master"),
					Repo: repository,
				},
				Head: &github.PullRequestBranch{
					SHA:  github.String("1235"),
					Ref:  github.String("feature"),
					Repo: repository,
				},
			}

			BeforeEach(func() {
				pullRequests.
					On("Get", repositoryOwner, repositoryName, issueNumber).
					Return(pr, nil, nil)
			})

			ItSquashesPR(context, pr)
		})
	})
})

var ItSquashesPR = func(context WebhookTestContext, pr *github.PullRequest) {
	var (
		handle = context.Handle

		responseRecorder *httptest.ResponseRecorder
		repositories     *MockRepositories
		git              *MockGit
		repo             *MockRepo

		baseRef = *pr.Base.Ref
		headRef = *pr.Head.Ref
		headSHA = *pr.Head.SHA
	)

	BeforeEach(func() {
		responseRecorder = *context.ResponseRecorder
		repositories = *context.Repositories
		git = *context.Git

		repo = new(MockRepo)
		git.
			On("GetUpdatedRepo", sshURL, repositoryOwner, repositoryName).
			Return(repo, nil)
	})

	AfterEach(func() {
		repo.AssertExpectations(GinkgoT())
	})

	Context("with autosquash failing", func() {
		BeforeEach(func() {
			repo.
				On("RebaseAutosquash", baseRef, headSHA).
				Return(errors.New("merge conflict"))
		})

		It("reports the failure", func() {
			repositories.
				On("CreateStatus", repositoryOwner, repositoryName, headSHA, mock.MatchedBy(func(status *github.RepoStatus) bool {
					return *status.State == "failure" && *status.Context == "review/squash"
				})).
				Return(nil, nil, nil)

			handle()

			Expect(responseRecorder.Code).To(Equal(http.StatusOK))
		})
	})

	Context("with autosquash succeeding", func() {
		BeforeEach(func() {
			repo.
				On("RebaseAutosquash", baseRef, headSHA).
				Return(nil)
		})

		It("pushes the squashed changes, reports status", func() {
			repo.
				On("ForcePushHeadTo", headRef).
				Return(nil)

			handle()
		})
	})
}
