import { Inject, Service, Container } from 'typedi'
import { Server as HttpServer } from 'http'
import { Server } from 'ws'
import { createServer } from './utils'
import { bootstrap } from 'ws-decorators'
import { join } from 'path'
import { Packet } from './types'
import Client from './client'
import { logger } from './utils/logger'
import { BrokerService } from './services/broker.service'
import { SessionManager } from './managers/session.manager'
import { Opcodes } from '@sakuraapp/common'
import Database from './database'
import { RoomService } from './services/room.service'

@Service()
export default class App {
    public readonly address = '0.0.0.0'
    public readonly port = Number(process.env.PORT) || 9000

    public app: HttpServer
    public server: Server

    @Inject()
    public database: Database
    
    @Inject()
    public sessionManager: SessionManager

    @Inject()
    public brokerService: BrokerService

    @Inject()
    public roomService: RoomService

    listen(): Promise<void> {
        return new Promise((resolve) => {
            this.app.listen(this.port, this.address, () => {
                resolve()
            })
        })
    }

    async init() {
        await this.database.init()
        await this.brokerService.init()
        this.roomService.init()

        this.app = createServer()
        this.server = new Server({ server: this.app })

        const manager = bootstrap(this.server, {
            directory: join(__dirname, 'controllers'),
            initialize(generator) {
                return Container.get(generator)
            },
            getClient: (socket) => {
                const client = new Client(socket)

                this.sessionManager.add(client)

                socket.on('close', () => {
                    manager.handle(Opcodes.DISCONNECT, null, client)

                    this.sessionManager.remove(client)
                })

                return client
            },
            getAction(data: Packet) {
                return data.op
            },
            getData(data: Packet) {
                return data.d
            },
            getParams(data: Packet) {
                return data.t
            },
        })
        
        await this.listen()

        logger.info(`Gateway listening on port ${this.port}`)
    }
}