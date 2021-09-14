import { Broker, BrokerHandler } from '@sakuraapp/shared'
import { Opcodes } from '@sakuraapp/common'
import { Inject, Service } from 'typedi'
import { StoreService } from './store.service'
import { MessageTarget } from '~/types'
import { EventEmitter } from 'stream'

@Service()
export class BrokerService {
    @Inject()
    storeService: StoreService

    private broker: Broker

    private actions: EventEmitter

    init() {
        this.broker = new Broker({
            client: this.storeService.store,
        })
    }

    dispatch<T = unknown>(message: T, target: MessageTarget): Promise<number> {
        const { roomId, userId } = target

        let channel: string
        
        if (roomId) {
            channel = `room.${roomId}`
        } else if (userId) {
            channel = `user.${userId}`
        }

        return this.broker.publish<T>(channel, message)
    }

    subscribe<T>(channel: string, handler: BrokerHandler<T>) {
        return this.broker.subscribe(channel, handler)
    }

    unsubscribe<T>(channel: string, handler: BrokerHandler<T>) {
        return this.broker.unsubscribe(channel, handler)
    }
}
