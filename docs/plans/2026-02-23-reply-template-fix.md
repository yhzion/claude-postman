# Template Forward → Reply 전환 구현 계획

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Gmail Forward에서 In-Reply-To 헤더가 누락되어 새 세션 생성이 실패하는 버그를 Reply 방식으로 전환하여 수정한다.

**Architecture:** `Poll()`에 자기수신 템플릿 필터를 추가하고, `SendTemplate()` 안내 문구를 Reply로 변경한다. 기존 `isTemplateRef()` 로직은 Reply의 In-Reply-To 헤더와 호환되므로 변경 불필요.

**Tech Stack:** Go, testify, SQLite

---

### Task 1: 자기수신 템플릿 필터링

**Files:**
- Modify: `internal/email/email.go:64-69` (Poll 메서드 내 sender 필터 이후)
- Test: `internal/email/email_test.go`

**Step 1: Write the failing test**

`email_test.go`에 자기수신 필터링 테스트 추가:

```go
t.Run("ignores self-received template email", func(t *testing.T) {
    smtp := &mockSMTPSender{}
    imapMock := &mockIMAPClient{}
    m, _ := testMailer(t, imapMock, smtp)

    // Send template to create DB record
    messageID, err := m.SendTemplate()
    require.NoError(t, err)

    // Simulate the template email arriving back via IMAP
    imapMock.emails = []*RawEmail{
        {
            From:      "user@example.com",
            Subject:   "[claude-postman] New Session",
            Body:      "How to create a new Claude Code session...",
            MessageID: messageID,
            UID:       1,
        },
    }

    msgs, err := m.Poll()
    require.NoError(t, err)
    assert.Empty(t, msgs, "self-received template should be filtered out")
    assert.Contains(t, imapMock.marked, imap.UID(1), "should still mark as read")
})
```

**Step 2: Run test to verify it fails**

Run: `cd /home/feel_so_good/datamaker/claude-postman && go test ./internal/email/ -run "TestPoll/ignores_self-received_template" -v`
Expected: FAIL — 현재 코드는 자기수신 템플릿을 필터링하지 않아 msgs가 1개 반환됨

**Step 3: Write minimal implementation**

`email.go`의 `Poll()` 메서드에서 sender 필터 이후, template ref 체크 이전에 추가:

```go
// Filter out self-received template emails
if ok, _ := m.store.IsValidTemplateRef(raw.MessageID); ok {
    slog.Debug("ignoring self-received template", "message_id", raw.MessageID)
    if markErr := client.MarkRead(raw.UID); markErr != nil {
        slog.Warn("failed to mark template as read", "uid", raw.UID, "error", markErr)
    }
    continue
}
```

삽입 위치: line 69 (`slog.Debug("ignoring email from non-authorized sender"...)` 블록 다음, `msg := &IncomingMessage{` 이전)

**Step 4: Run test to verify it passes**

Run: `cd /home/feel_so_good/datamaker/claude-postman && go test ./internal/email/ -run "TestPoll/ignores_self-received_template" -v`
Expected: PASS

**Step 5: Run all existing tests to verify no regression**

Run: `cd /home/feel_so_good/datamaker/claude-postman && go test ./internal/email/ -v`
Expected: All PASS

**Step 6: Commit**

```bash
git add internal/email/email.go internal/email/email_test.go
git commit -m "fix: filter self-received template emails in IMAP poll"
```

---

### Task 2: 템플릿 안내 문구 Forward → Reply 변경

**Files:**
- Modify: `internal/email/email.go:165-191` (SendTemplate 메서드의 templateBody)
- Test: `internal/email/email_test.go`

**Step 1: Write the failing test**

기존 `TestSendTemplate` 테스트에 본문 내용 검증 추가:

```go
t.Run("template body instructs reply not forward", func(t *testing.T) {
    smtp := &mockSMTPSender{}
    m, _ := testMailer(t, imapMock, smtp)

    _, err := m.SendTemplate()
    require.NoError(t, err)

    require.Len(t, smtp.sent, 1)
    body := smtp.sent[0].body
    assert.NotContains(t, body, "FORWARD")
    assert.NotContains(t, body, "forward")
    assert.Contains(t, body, "REPLY")
})
```

**Step 2: Run test to verify it fails**

Run: `cd /home/feel_so_good/datamaker/claude-postman && go test ./internal/email/ -run "TestSendTemplate/template_body_instructs_reply" -v`
Expected: FAIL — 현재 본문에 "FORWARD" 포함

**Step 3: Update template body**

`email.go`의 `SendTemplate()` 내 `templateBody`를 다음으로 교체:

```go
templateBody := `How to create a new Claude Code session
========================================

IMPORTANT — Do NOT change:
  - The subject line (must contain [claude-postman])
  - You must REPLY to this email, not compose a new one or forward
  - Send to yourself (your own email address)
  - Keep "Directory:" and "Model:" keywords exactly as written

You CAN edit:
  - The path after "Directory:" (e.g. ~/my-project)
  - The model after "Model:" — sonnet | opus | haiku
  - Replace "(Write your task here)" with your task

────────────────────────────────────

Directory: ~
Model: sonnet

(Write your task here)

────────────────────────────────────

Tips:
  - You can reply to this email multiple times
    — each reply creates a new session
  - A fresh template is sent every time the server starts`
```

**Step 4: Run test to verify it passes**

Run: `cd /home/feel_so_good/datamaker/claude-postman && go test ./internal/email/ -run "TestSendTemplate" -v`
Expected: All PASS

**Step 5: Run all tests**

Run: `cd /home/feel_so_good/datamaker/claude-postman && go test ./internal/email/ -v`
Expected: All PASS

**Step 6: Commit**

```bash
git add internal/email/email.go internal/email/email_test.go
git commit -m "fix: change template instructions from forward to reply"
```

---

### Task 3: 테스트명 정리 + 아키텍처 문서 동기화

**Files:**
- Modify: `internal/email/email_test.go:221` (테스트명)
- Modify: `docs/architecture/05-email.md:88-131` (섹션 2.4)

**Step 1: Rename test**

`email_test.go`의 line 221:
```go
// 변경 전
t.Run("detects new session via template forward", func(t *testing.T) {
// 변경 후
t.Run("detects new session via template reply", func(t *testing.T) {
```

**Step 2: Update architecture doc**

`docs/architecture/05-email.md` 섹션 2.4에서:
- "포워드" → "답장" (4곳)
- "Forward" → "Reply" (해당되는 곳)
- 설명 흐름에서 "사용자가 템플릿 이메일 포워드" → "사용자가 템플릿 이메일에 답장"

**Step 3: Run all tests**

Run: `cd /home/feel_so_good/datamaker/claude-postman && go test ./internal/email/ -v`
Expected: All PASS

**Step 4: Commit**

```bash
git add internal/email/email_test.go docs/architecture/05-email.md
git commit -m "docs: sync architecture doc and test names for reply-based template"
```

---

### Task 4: 전체 검증

**Step 1: Run full test suite**

Run: `cd /home/feel_so_good/datamaker/claude-postman && go test ./... -v`
Expected: All PASS

**Step 2: Run linter**

Run: `cd /home/feel_so_good/datamaker/claude-postman && golangci-lint run ./...`
Expected: No issues

**Step 3: Build**

Run: `cd /home/feel_so_good/datamaker/claude-postman && go build ./cmd/claude-postman/`
Expected: Build succeeds
