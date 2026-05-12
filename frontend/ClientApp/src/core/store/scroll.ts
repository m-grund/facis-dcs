import { defineStore } from 'pinia'
import { ref } from 'vue'
import { useRouter } from 'vue-router'

export const useScrollStore = defineStore('scroll', () => {
  const router = useRouter()
  const scrollContainer = ref<HTMLElement | null>(null)

  function scrollToTop() {
    scrollContainer.value?.scrollTo({
      top: 0,
      behavior: 'smooth',
    })
  }

  return { scrollContainer, scrollToTop }
})
