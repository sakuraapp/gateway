import { Service } from 'typedi'
import { Store } from './store.store'

@Service()
export class RoomStore extends Store {
    public readonly name = 'rooms'
}
