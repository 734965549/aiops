// @vitest-environment jsdom
import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import QueuedMessageList from './QueuedMessageList.vue'
import type { QueuedChatMessage } from '../composables/chatSessionState'

vi.mock('@arco-design/web-vue', () => ({
  Button: {
    emits: ['click'],
    template: '<button @click="$emit(\'click\')"><slot /></button>'
  }
}))

function makeMessage(queueId: string, content: string): QueuedChatMessage {
  return {
    queue_id: queueId,
    session_id: 'session-1',
    content,
    images: [],
    context: {},
    client_message_id: `client-${queueId}`
  }
}

describe('QueuedMessageList', () => {
  it('emits prioritize, remove, and clear actions', async () => {
    const wrapper = mount(QueuedMessageList, {
      props: {
        messages: [
          makeMessage('queue-1', '第一条'),
          makeMessage('queue-2', '第二条'),
          makeMessage('queue-3', '第三条')
        ]
      }
    })

    const items = wrapper.findAll('.queue-item')
    expect(items).toHaveLength(3)
    expect(items[0].text()).not.toContain('置顶')

    await items[1].findAll('button')[0].trigger('click')
    expect(wrapper.emitted('prioritize')).toEqual([['queue-2']])

    await items[1].findAll('button')[1].trigger('click')
    expect(wrapper.emitted('remove')).toEqual([['queue-2']])

    await wrapper.find('.queue-header button').trigger('click')
    expect(wrapper.emitted('clear')).toEqual([[]])
  })
})
