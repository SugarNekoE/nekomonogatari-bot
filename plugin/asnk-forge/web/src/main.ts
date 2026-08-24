import { mount } from 'svelte';
import App from './App.svelte';
import 'bootstrap/dist/css/bootstrap.min.css';
import './styles.css';

mount(App, { target: document.getElementById('app')! });
