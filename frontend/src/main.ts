import { mount } from 'svelte'
import 'sve-ui/theme.css'
import './styles.css'
import App from './App.svelte'

const app = mount(App, {
  target: document.getElementById('app')!,
})

export default app
