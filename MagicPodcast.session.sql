select * from podcasts
where title = '三五环'

select * from podcasts_tags
where podcast_id in (select id from podcasts where title = '三五环')

select count(1) from podcasts_tags


select e.title from
((select b.title, a.tag_id from podcasts_tags a join podcasts b on a.podcast_id = b.id) as c join tags d on c.tag_id = d.id) as e
where e.name = '儿童教育'

select count(distinct tag_id) from podcasts_tags

update tags
set name = '商业' where name = '商务'