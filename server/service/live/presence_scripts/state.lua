-- All session keys share one hash tag. Every loop is bounded by 4 or the supplied <=100 batch.
-- Preflight key types BEFORE mutation: Redis scripts are atomic but do not roll back errors.
local types={'hash','hash','zset','zset','zset','hash','set','zset','stream','zset','hash'}
for i,k in ipairs(KEYS) do local t=redis.call('TYPE',k).ok;if t~='none' and t~=types[i] then return {'corrupt'} end end
local a=cjson.decode(ARGV[1]);local t=redis.call('TIME');local now=t[1]*1000+math.floor(t[2]/1000)
local function str(n)return string.format('%.0f',n)end
local function num(f)return tonumber(redis.call('HGET',KEYS[1],f) or '0')end
local function touch()for _,k in ipairs(KEYS) do redis.call('EXPIRE',k,604800)end end
local function dirty(c,m)
 redis.call('HSET',KEYS[2],c,cjson.encode(m));redis.call('ZADD',KEYS[8],'NX',now,c)
end
local function revision()
 local n=redis.call('HINCRBY',KEYS[1],'revision',1);redis.call('HSET',KEYS[1],'updated',str(now));return n
end
local function event(typ,c,m,extra)
 redis.call('XADD',KEYS[9],'*','type',typ,'connection',c,'actor',m.actor or '', 'at',str(now),'payload',extra or '{}')
end
local function actor(key)
 local raw=redis.call('HGET',KEYS[6],key);if raw then return cjson.decode(raw) end
 return {connections={},timeline=0,duration=0,lastActive=0,activity='heartbeat',index=''}
end
local function index(u,key,at,typ)
 if string.sub(key,1,2)=='u:' then
  if u.index~='' then redis.call('ZREM',KEYS[10],u.index)end
  u.index=string.format('%016.0f',9007199254740991-at)..':'..key
  redis.call('ZADD',KEYS[10],0,u.index)
 end
 u.lastActive=at;u.activity=typ
end
local function online(u,key)
 local maxexp=0;local alive={}
 for _,c in ipairs(u.connections) do
  local exp=tonumber(redis.call('ZSCORE',KEYS[3],c) or '0')
  if exp>now then table.insert(alive,c);maxexp=math.max(maxexp,exp)end
 end
 u.connections=alive
 local z=KEYS[5];if string.sub(key,1,2)=='u:' then z=KEYS[4] end
 if maxexp>now then redis.call('ZADD',z,maxexp,key)else
  redis.call('ZREM',z,key)
  if u.index~='' then redis.call('ZREM',KEYS[10],u.index);u.index='' end
 end
 redis.call('HSET',KEYS[6],key,cjson.encode(u))
end
local function duration(m,u,at,playing,visible)
 -- Conservative union timeline: late overlapping intervals never add twice.
 local last=tonumber(m.last);at=math.max(last,at)
 if m.playing and m.visible and playing and visible then
  local start=math.max(last,at-a.maxDelta)
  m.duration=m.duration+math.max(0,at-start)
  local delta=math.max(0,at-math.max(start,u.timeline))
  u.duration=u.duration+delta;redis.call('HINCRBY',KEYS[1],'watch',delta)
  u.timeline=math.max(u.timeline,at)
 end
 m.last=at
end
local function result(m,accepted)
 local members=redis.call('ZCOUNT',KEYS[4],'('..str(now),'+inf')
 local guests=redis.call('ZCOUNT',KEYS[5],'('..str(now),'+inf')
 local conns=redis.call('ZCOUNT',KEYS[3],'('..str(now),'+inf')
 if members+guests>conns then return {'corrupt'}end
 local oldest=redis.call('ZRANGE',KEYS[3],0,0,'WITHSCORES');local lag=0
 if #oldest>0 then lag=math.max(0,now-tonumber(oldest[2]))end
 return {'ok',cjson.encode({connectionCount=str(conns),memberCount=str(members),guestCount=str(guests),totalOnlineCount=str(members+guests),onlineCount=str(members),viewCount=str(num('view')),viewerCount=str(num('viewer')),peakOnlineCount=str(num('peak')),likeCount=str(num('likes')),totalWatchDurationMs=str(num('watch')),updatedAt=now,pendingCount=str(redis.call('ZCARD',KEYS[8])+redis.call('XLEN',KEYS[9])),cleanupLagMs=str(lag),checkpointAt=num('committed'),dataLost=num('lost')==1,closed=num('closed')>0,viewerSessionDurationMs=str(m and actor(m.actor).duration or 0),meta=m or {},acceptedCount=accepted or ''})}
end
if a.op=='init' then
 if redis.call('EXISTS',KEYS[1])==0 then
  redis.call('HSET',KEYS[1],'epoch',a.epoch,'room',a.room,'revision','0','created',str(now),'updated',str(now),'committed',str(now),'closed','0','view',a.view,'viewer',a.viewer,'peak',a.peak,'likes',a.likes,'watch',a.watch,'lost',a.lost)
  touch()
 end
 return {'ok'}
end
if redis.call('HGET',KEYS[1],'epoch')~=a.epoch then return {'expired'}end
if a.op=='stats' then return result()end
if a.op=='ack' then
 for _,v in ipairs(a.visits) do
  local raw=redis.call('HGET',KEYS[2],v.c)
  if raw and cjson.decode(raw).revision==v.revision then redis.call('ZREM',KEYS[8],v.c)end
 end
 for _,id in ipairs(a.events) do redis.call('XDEL',KEYS[9],id)end
 redis.call('HSET',KEYS[1],'committed',str(now));touch();return {'ok'}
end
if a.op=='close' then
 if num('closed')==0 then redis.call('HSET',KEYS[1],'closed',str(a.cutoff));revision();event('room.session.ended','',{actor=''},'{}')end
 touch();return {'ok'}
end
if a.op=='finalize' then
 if redis.call('ZCARD',KEYS[3])>0 or redis.call('ZCARD',KEYS[8])>0 or redis.call('XLEN',KEYS[9])>0 then return {'busy'}end
 for _,k in ipairs(KEYS)do redis.call('EXPIRE',k,86400)end
 return {'ok'}
end
if num('revision')>9007199253740991 or num('watch')>9007198254740991 or num('likes')>9007199253740991 then return {'capacity'}end
local function finish(c,m,reason,at)
 local u=actor(m.actor);duration(m,u,at,m.playing,m.visible)
 m.closed=true;m.reason=reason;m.left=at;m.revision=revision()
 redis.call('ZREM',KEYS[3],c);online(u,m.actor);dirty(c,m)
 event(reason=='timeout' and 'room.viewer.timeout' or 'room.viewer.left',c,m,cjson.encode({reason=reason}))
end
if a.op=='expire' then
 -- Reserve room for the bounded cleanup events before mutating anything.
 if redis.call('XLEN',KEYS[9])+a.batch>20000 then return {'capacity'}end
 local cutoff=now;if num('closed')>0 then cutoff=9007199254740991 end
 local cs=redis.call('ZRANGEBYSCORE',KEYS[3],'-inf',str(cutoff),'LIMIT',0,a.batch)
 for _,c in ipairs(cs)do
  local raw=redis.call('HGET',KEYS[2],c);if not raw then return {'corrupt'}end
  local m=cjson.decode(raw)
  if not m.closed then
   local at=math.min(now,m.last+a.maxDelta);local reason='timeout'
   if num('closed')>0 then at=math.min(at,num('closed'));reason='session_ended' end
   finish(c,m,reason,math.max(m.last,at))
  else redis.call('ZREM',KEYS[3],c)end
 end
 touch();return {'ok',str(#cs)}
end
local raw=redis.call('HGET',KEYS[2],a.connection)
local m=nil;if raw then m=cjson.decode(raw)end
if a.op=='open' and m then
 if m.actor~=a.actor or m.closed or tonumber(redis.call('ZSCORE',KEYS[3],a.connection) or '0')<=now then return {'expired'}end
 return result(m)
end
if a.op=='leave' and m and m.actor==a.actor and m.closed then return result(m)end
if a.op~='open' and (not m or m.actor~=a.actor or m.closed or tonumber(redis.call('ZSCORE',KEYS[3],a.connection) or '0')<=now) then return {'expired'}end
if a.op~='leave' and num('closed')>0 then return {'ended'}end
if a.op=='open' then
 if now-num('committed')>60000 then return {'capacity'}end
 if redis.call('HLEN',KEYS[2])>=100000 or redis.call('ZCARD',KEYS[8])>=10000 or redis.call('XLEN',KEYS[9])>=19900 or redis.call('ZCOUNT',KEYS[3],'('..str(now),'+inf')>=a.roomLimit then return {'capacity'}end
 local u=actor(a.actor);online(u,a.actor)
 if #u.connections>=4 then return {'limited'}end
 m={actor=a.actor,joined=now,last=now,seq='0',likeSeq='0',likes=0,duration=0,playing=false,visible=false,closed=false,reason='',left=0,revision=revision(),authAt=now,ip=a.ip,history=a.history,historyVersion=a.historyVersion,historyEpoch=a.historyEpoch}
 table.insert(u.connections,a.connection)
 redis.call('ZADD',KEYS[3],now+a.ttl,a.connection);index(u,a.actor,now,'entered');online(u,a.actor)
 redis.call('HINCRBY',KEYS[1],'view',1)
 if redis.call('SADD',KEYS[7],a.actor)==1 then redis.call('HINCRBY',KEYS[1],'viewer',1)end
 local n=redis.call('ZCOUNT',KEYS[4],'('..str(now),'+inf')+redis.call('ZCOUNT',KEYS[5],'('..str(now),'+inf')
 if n>num('peak')then redis.call('HSET',KEYS[1],'peak',str(n))end
 dirty(a.connection,m);event('room.viewer.entered',a.connection,m);touch();return result(m)
end
if a.op=='auth' then
 m.authAt=now;redis.call('HSET',KEYS[2],a.connection,cjson.encode(m));return {'ok'}
end
if a.op=='activity' then
 local u=actor(m.actor);if a.at>u.lastActive then index(u,m.actor,a.at,a.activity);redis.call('HSET',KEYS[6],m.actor,cjson.encode(u))end
 return {'ok'}
end
if a.op=='like' then
 local receiptKey=a.actor..':'..a.request
 local receipt=redis.call('HGET',KEYS[11],receiptKey)
 if receipt then
  local r=cjson.decode(receipt)
  if r.connection~=a.connection or r.seq~=a.sequence or r.count~=a.count then return {'conflict'}end
  return result(m,r.count)
 end
 if tonumber(a.sequence)<=tonumber(m.likeSeq)then return {'sequence'}end
 if redis.call('HLEN',KEYS[11])>=100000 or redis.call('XLEN',KEYS[9])>=19900 then return {'capacity'}end
 local u=actor(m.actor);local sec=math.floor(now/1000);local minute=math.floor(now/60000)
 if u.second~=sec then u.second=sec;u.secondCount=0 end
 if u.minute~=minute then u.minute=minute;u.minuteCount=0 end
 if m.likeSecond~=sec then m.likeSecond=sec;m.likeSecondCount=0 end
 local n=tonumber(a.count)
 if u.secondCount+n>a.likeSecond or u.minuteCount+n>a.likeMinute or m.likeSecondCount+n>a.likeSecond then return {'limited'}end
 u.secondCount=u.secondCount+n;u.minuteCount=u.minuteCount+n;m.likeSecondCount=m.likeSecondCount+n
 m.likeAt=now;m.likes=(m.likes or 0)+n;m.likeSeq=a.sequence;m.revision=revision();index(u,m.actor,now,'liked');redis.call('HSET',KEYS[6],m.actor,cjson.encode(u))
 redis.call('HINCRBY',KEYS[1],'likes',n);redis.call('HSET',KEYS[11],receiptKey,cjson.encode({connection=a.connection,seq=a.sequence,count=a.count}))
 dirty(a.connection,m);touch();return result(m,a.count)
end
if tonumber(a.sequence)<tonumber(m.seq)then return {'sequence'}end
if tonumber(a.sequence)==tonumber(m.seq)then
 if a.op=='leave' or m.playing~=a.playing or m.visible~=a.visible then return {'conflict'}end
 return result(m)
end
if redis.call('XLEN',KEYS[9])>=19900 then return {'capacity'}end
local u=actor(m.actor)
if a.op=='leave' then
 m.seq=a.sequence;finish(a.connection,m,a.reason,now)
else
 duration(m,u,now,a.playing,a.visible);m.seq=a.sequence;m.playing=a.playing;m.visible=a.visible;m.revision=revision()
 redis.call('ZADD',KEYS[3],now+a.ttl,a.connection);index(u,m.actor,now,'heartbeat');online(u,m.actor);dirty(a.connection,m)
end
touch();return result(m)
