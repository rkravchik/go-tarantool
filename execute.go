package tarantool

import (
	"context"
)

func (conn *Connection) doExecute(ctx context.Context, r *request) ([][]interface{}, error) {
	var err error

	requestID := conn.nextID()

	pp := packIproto(0, requestID)
	defer pp.Release()

	if pp.code, err = r.query.Pack(conn.packData, &pp.buffer); err != nil {
		return nil, &QueryError{Code: ErrInvalidMsgpack, error: err}
	}

	if oldRequest := conn.requests.Put(requestID, r); oldRequest != nil {
		oldRequest.replyChan <- &Result{
			Error: NewConnectionError(conn, "shred old requests"), // wtf?
		}
		close(oldRequest.replyChan)
	}

	select {
	case conn.writeChan <- pp:
	case <-ctx.Done():
		conn.requests.Pop(requestID)
		return nil, NewContextError(ctx, conn, "send error")
	case <-conn.exit:
		return nil, ConnectionClosedError(conn)
	}

	var res *Result
	select {
	case res = <-r.replyChan:
	case <-ctx.Done():
		return nil, NewContextError(ctx, conn, "recv error")
	case <-conn.exit:
		return nil, ConnectionClosedError(conn)
	}

	return res.Data, res.Error
}

func (conn *Connection) Exec(ctx context.Context, q Query) ([][]interface{}, error) {
	var cancel context.CancelFunc = func() {}
	defer cancel()

	request := &request{
		query:     q,
		replyChan: make(chan *Result, 1),
	}

	if _, ok := ctx.Deadline(); !ok && conn.queryTimeout != 0 {
		ctx, cancel = context.WithTimeout(ctx, conn.queryTimeout)
	}
	return conn.doExecute(ctx, request)
}

func (conn *Connection) Execute(q Query) ([][]interface{}, error) {
	return conn.Exec(context.Background(), q)
}
