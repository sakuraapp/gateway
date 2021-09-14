import { Service } from 'typedi'
import { Store } from './store.store'

@Service()
export class SessionStore extends Store {
    public readonly name = 'sessions'
}
