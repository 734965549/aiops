// @vitest-environment jsdom
import { mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import AnswerCard from './AnswerCard.vue'
import type { ChatAnswer } from '@/api/ai'
import { copyText } from '../composables/useCopy'

vi.mock('../composables/useCopy', () => ({ copyText: vi.fn() }))

const stubs = {
  ATag: { template: '<span><slot /></span>' },
  AButton: { template: '<button><slot /></button>' },
  ATooltip: { template: '<span><slot /></span>' },
}

beforeEach(() => {
  vi.clearAllMocks()
})

function answerWithDiagnosis(diagnosis: ChatAnswer['diagnosis']): ChatAnswer {
  return {
    summary: 'test summary',
    evidence: [],
    impact: [],
    recommendations: [],
    risk_level: 'high',
    require_confirm: true,
    actions: [],
    rollback: '无变更动作',
    uncertainties: [],
    insufficient_context: false,
    diagnosis,
  }
}

const pendingRootStatement = '已确认存在空闲长事务；应用原因未定'

function answerWithPending(): ChatAnswer {
  return {
    ...answerWithDiagnosis({
      facts: [],
      root_cause: { status: 'confirmed', code: 'idle_in_transaction', statement: pendingRootStatement, evidence_ids: [] },
      hypotheses: [],
      missing_evidence: ['application_logs'],
    }),
    uncertainties: [pendingRootStatement, '缺失证据：application_logs', '还需核对连接池', '还需核对连接池'],
  }
}

describe('AnswerCard diagnosis', () => {
  it('renders suspected root cause with empty hypothesis arrays', () => {
    const wrapper = mount(AnswerCard, {
      props: {
        answer: answerWithDiagnosis({
          facts: [{ statement: '会话 42；Query ID query-42', evidence_id: 'ev-1', pointer: '/sessions/0' }],
          root_cause: { status: 'suspected', code: 'idle_in_transaction', statement: '存在空闲长事务', evidence_ids: ['ev-1'] },
          hypotheses: [],
          missing_evidence: [],
        }),
      },
      global: { stubs },
    })
    expect(wrapper.text()).toContain('疑似根因（未人工确认）')
    expect(wrapper.text()).toContain('query-42')
    expect(wrapper.text()).toContain('ev-1')
  })

  it('renders confirmed category without claiming the application reason', () => {
    const wrapper = mount(AnswerCard, {
      props: {
        answer: answerWithDiagnosis({
          facts: [{ statement: '会话 42；脱敏 SQL 摘要 UPDATE orders SET status = ?', evidence_id: 'ev-1', pointer: '/sessions/0' }],
          root_cause: { status: 'confirmed', code: 'idle_in_transaction', statement: '已确认存在空闲长事务；应用为何未结束事务仍未定', evidence_ids: ['ev-1'] },
          hypotheses: [],
          missing_evidence: ['application_logs'],
        }),
      },
      global: { stubs },
    })
    expect(wrapper.text()).toContain('分类已确认')
    expect(wrapper.text()).toContain('应用为何未结束事务仍未定')
    expect(wrapper.text()).toContain('UPDATE orders SET status = ?')
    expect(wrapper.text()).toContain('application_logs')
    expect(wrapper.text()).not.toContain('已确认根因')
  })

  it('renders undetermined root cause and missing evidence', () => {
    const wrapper = mount(AnswerCard, {
      props: {
        answer: answerWithDiagnosis({
          facts: [],
          root_cause: { status: 'undetermined', code: 'undetermined', statement: '根因未定', evidence_ids: [] },
          hypotheses: [{ code: 'lock_wait', statement: '可能存在锁等待', missing_evidence: ['postgres_activity'] }],
          missing_evidence: ['postgres_activity', 'postgres_wait_events'],
        }),
      },
      global: { stubs },
    })
    expect(wrapper.text()).toContain('原因未定')
    expect(wrapper.text()).toContain('待验证假设')
    expect(wrapper.text()).toContain('postgres_activity')
    expect(wrapper.text()).toContain('postgres_wait_events')
  })
})

describe('AnswerCard pending items and footer', () => {
  it('merges and deduplicates missing evidence and uncertainties', () => {
    const wrapper = mount(AnswerCard, {
      props: { answer: answerWithPending() },
      global: { stubs },
    })

    expect(wrapper.find('.answer-uncertainty h4').text()).toBe('待确认事项')
    expect(wrapper.text()).not.toContain('不确定性说明')
    expect(wrapper.text()).not.toContain('缺失 / 失败证据')
    expect(wrapper.text().split(pendingRootStatement)).toHaveLength(2)
    expect(wrapper.text().split('application_logs')).toHaveLength(2)
    expect(wrapper.text().split('还需核对连接池')).toHaveLength(2)
  })

  it('places copy and feedback actions at the bottom of the answer', () => {
    const wrapper = mount(AnswerCard, {
      props: { answer: answerWithDiagnosis(undefined), messageId: 'message-1' },
      global: { stubs },
    })

    const footer = wrapper.find('.answer-card').element.lastElementChild
    expect(footer?.classList.contains('answer-feedback--bottom')).toBe(true)
    expect(footer?.textContent).toContain('复制全文')
    expect(footer?.textContent).toContain('有效')
    expect(wrapper.find('.answer-head').text()).not.toContain('复制全文')
  })

  it('uses the same deduplicated pending section in copied markdown', async () => {
    const wrapper = mount(AnswerCard, {
      props: { answer: answerWithPending(), messageId: 'message-1' },
      global: { stubs },
    })

    await wrapper.find('.answer-feedback--bottom button').trigger('click')
    const markdown = vi.mocked(copyText).mock.calls[0][0]
    expect(markdown).toContain('## 待确认事项')
    expect(markdown).not.toContain('## 不确定性')
    expect(markdown).not.toContain('## 缺失 / 失败证据')
    expect(markdown.split(pendingRootStatement)).toHaveLength(2)
    expect(markdown.split('application_logs')).toHaveLength(2)
    expect(markdown.split('还需核对连接池')).toHaveLength(2)
  })
})
