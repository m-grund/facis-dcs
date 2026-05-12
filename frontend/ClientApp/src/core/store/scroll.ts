import { defineStore } from 'pinia'
import { ref } from 'vue'

export const useScrollStore = defineStore('scroll', () => {
  const scrollContainer = ref<HTMLElement | null>(null)

  function scrollToTop() {
    scrollContainer.value?.scrollTo({
      top: 0,
      behavior: 'smooth',
    })
  }

  return { scrollContainer, scrollToTop }
})
