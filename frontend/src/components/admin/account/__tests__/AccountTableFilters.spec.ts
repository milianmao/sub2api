import { computed, defineComponent } from 'vue'
import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

import AccountTableFilters from '../AccountTableFilters.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key
  })
}))

const SelectStub = defineComponent({
  name: 'SelectStub',
  props: {
    modelValue: {
      type: [String, Number, Boolean, null],
      default: ''
    },
    options: {
      type: Array,
      default: () => []
    }
  },
  setup(props) {
    const optionValues = computed(() =>
      (props.options as Array<{ value: string | number | boolean | null }>).map((option) => option.value)
    )

    return {
      optionValues
    }
  },
  template: '<div class="select-stub" :data-option-values="JSON.stringify(optionValues)"></div>'
})

describe('AccountTableFilters', () => {
  it('includes upstream and service account in type filter options', () => {
    const wrapper = mount(AccountTableFilters, {
      props: {
        searchQuery: '',
        filters: {
          platform: '',
          type: '',
          status: '',
          privacy_mode: '',
          group: ''
        },
        groups: []
      },
      global: {
        stubs: {
          Select: SelectStub,
          SearchInput: true
        }
      }
    })

    const typeFilter = wrapper.findAll('.select-stub')[1]
    const optionValues = JSON.parse(typeFilter.attributes('data-option-values') || '[]')

    expect(optionValues).toContain('upstream')
    expect(optionValues).toContain('service_account')
  })
})
