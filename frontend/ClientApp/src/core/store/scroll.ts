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

  function addGutter() {
    scrollContainer.value?.classList.add('scrollbar-gutter-stable')
  }

  function removeGutter() {
    scrollContainer.value?.classList.remove('scrollbar-gutter-stable')
  }

  return { scrollContainer, scrollToTop, addGutter, removeGutter }
})
