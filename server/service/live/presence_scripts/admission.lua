-- Account, HMAC(IP) and global reservations intentionally share an admission slot.
local t=redis.call('TIME');local now=t[1]*1000+math.floor(t[2]/1000)
for _,k in ipairs(KEYS)do local typ=redis.call('TYPE',k).ok;if typ~='none' and typ~='zset' then return -2 end end
if ARGV[1]=='release' then for _,k in ipairs(KEYS)do redis.call('ZREM',k,ARGV[2])end;return 1 end
for i,k in ipairs(KEYS)do
 local old=redis.call('ZRANGEBYSCORE',k,'-inf',now,'LIMIT',0,100)
 if #old>0 then redis.call('ZREM',k,unpack(old))end
 local exists=tonumber(redis.call('ZSCORE',k,ARGV[2]) or '0')>now
 if not exists and redis.call('ZCOUNT',k,'('..now,'+inf')>=tonumber(ARGV[3+i])then return -1 end
end
for _,k in ipairs(KEYS)do redis.call('ZADD',k,now+tonumber(ARGV[3]),ARGV[2]);redis.call('PEXPIRE',k,tonumber(ARGV[3])+60000)end
return 1
