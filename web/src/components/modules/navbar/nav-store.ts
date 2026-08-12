import { create } from 'zustand'
import { persist } from 'zustand/middleware'

export type NavItem = 'home' | 'channel' | 'group' | 'health' | 'model' | 'log' | 'setting'

const NAV_ORDER: NavItem[] = ['home', 'channel', 'group', 'health', 'model', 'log', 'setting']

interface NavState {
    activeItem: NavItem
    direction: number
    setActiveItem: (item: NavItem) => void
}

export const useNavStore = create<NavState>()(
    persist(
        (set, get) => ({
            activeItem: 'home',
            direction: 0,
            setActiveItem: (item) => {
                const { activeItem } = get()
                const currentIndex = NAV_ORDER.indexOf(activeItem)
                const newIndex = NAV_ORDER.indexOf(item)
                const direction = newIndex > currentIndex ? 1 : -1

                set({
                    activeItem: item,
                    direction
                })
            },
        }),
        {
            name: 'nav-storage',
        }
    )
)
