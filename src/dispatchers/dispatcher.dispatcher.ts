import { Opcodes } from '@sakuraapp/common'
import { Inject, Service } from 'typedi'
import Client from '~/client'
import { SessionManager } from '~/managers/session.manager'
import { BrokerService } from '~/services/broker.service'
import { MessageTarget, Packet } from '~/types'

@Service({ transient: true })
class Receiver {
    public count = 0 
    public channel: string
    public target: MessageTarget

    @Inject()
    private dispatcher: Dispatcher

    @Inject()
    private brokerService: BrokerService

    constructor() {
        this.handle = this.handle.bind(this)
    }

    async increment() {
        if (this.count++ === 0) {
            await this.brokerService.subscribe(this.channel, this.handle)
        }
    }

    async decrement() {
        if (--this.count === 0) {
            await this.brokerService.subscribe(this.channel, this.handle)
        }
    }

    handle<T>(data: Packet<T>) {
        this.dispatcher.localDispatch(data.op, data.d, this.target)
    }
}

@Service()
export class Dispatcher {
    @Inject()
    private brokerService: BrokerService

    @Inject()
    private sessionManager: SessionManager

    private receivers: Map<string, Receiver>

    createMessage<T = unknown>(type: Opcodes, data: T, time?: number): Packet<T> {
        return {
            op: type,
            d: data,
            t: time || Date.now(),
        }
    }

    async dispatch<T = unknown>(type: Opcodes, data: T, target: MessageTarget = {}, time?: number): Promise<void> {
        this.localDispatch(type, data, target, time)

        await this.remoteDispatch(type, data, target, time)
    }

    remoteDispatch<T = unknown>(type: Opcodes, data: T, target: MessageTarget = {}, time?: number): Promise<number> {
        const message = this.createMessage(type, data, time)

        return this.brokerService.dispatch<Packet<T>>(message, target)
    }

    localDispatch<T = unknown>(type: Opcodes, data: T, target: MessageTarget = {}, time?: number) {
        let clients: Client[]
        
        const { roomId, userId, ignored } = target

        if (roomId) {
            clients = this.sessionManager.getAllByRoomId(roomId)
        } else if (userId) {
            clients = this.sessionManager.getAllByUserId(userId)
        } else {
            clients = this.sessionManager.getAll()
        }

        const message = this.createMessage(type, data, time)

        for (const client of clients) {
            if (!ignored || !ignored.includes(client.session.id)) {
                client.send(message)
            }
        }
    }

    async register(target: MessageTarget) {
        const { roomId, userId } = target
        let channel: string

        if (roomId) {
            channel = `room.${roomId}`
        } else if (userId) {
            channel = `user.${userId}`
        }

        let receiver = this.receivers.get(channel)

        if (!receiver) {
            receiver = new Receiver()

            receiver.channel = channel
            receiver.target = target

            this.receivers.set(channel, receiver)
        }

        await receiver.increment()
    }

    async unregister(target: MessageTarget) {
        const { roomId, userId } = target
        let channel: string

        if (roomId) {
            channel = `room.${roomId}`
        } else if (userId) {
            channel = `user.${userId}`
        }

        let receiver = this.receivers.get(channel)

        if (receiver) {
            await receiver.decrement()
        }
    }
}
