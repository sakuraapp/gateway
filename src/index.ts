import 'reflect-metadata'
import 'module-alias/register'
import dotenv from 'dotenv'

dotenv.config()

import Container from 'typedi'
import App from './app'

import './config/jwt'

const app = Container.get(App)

app.init()
